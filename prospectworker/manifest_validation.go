// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func matchesManifestPolicy(job prospectjob.ManifestJob, config ManifestConfig) bool {
	c, p := job.Context, config.Policy
	return job.AuthorityID == config.AuthorityPin.AuthorityID && job.DescriptorID == config.AuthorityPin.DescriptorID && c.Network == config.AuthorityPin.Network && c.Genesis == config.AuthorityPin.Genesis && c.Version == p.Version && c.Network == p.Network && c.Genesis == p.Genesis && c.DNAVersion == p.DNAVersion && c.ProspectTarget == p.ProspectTarget
}

func validateManifestScan(scan ManifestScan, config ManifestConfig) error {
	if !manifestTemplateBounds(scan.Template) || scan.Version != Version2 || scan.NextNonce > 1<<32 || scan.Job.PublicKey != config.PublicKey || !matchesManifestPolicy(scan.Job, config) {
		return ErrCorrupt
	}
	candidate, err := prospectwork.BuildManifest(scan.Job, scan.Template, config.PayoutAddress, scan.ExtraNonce)
	if err != nil || !reflect.DeepEqual(candidate, scan.Candidate) {
		return ErrCorrupt
	}
	return nil
}

// Bound live adapter-owned values before copying or case normalization.
func manifestCandidateBounds(c prospectwork.Candidate) bool {
	return len(c.BlockHex) >= 162 && len(c.BlockHex) <= 2*wire.MaxBlockPayload && len(c.JobID) == 64 && len(c.PayoutAddress) <= 128 && len(c.Proof.MerkleBranch) <= 20 && len(c.Proof.Header) == 160 && len(c.Proof.Coinbase) <= 32768
}
func manifestWorkBlock(job prospectjob.ManifestJob, candidate prospectwork.Candidate, config ManifestConfig) (*wire.MsgBlock, error) {
	if !manifestCandidateBounds(candidate) || !manifestCanonicalProof(candidate.Proof) || candidate.BlockHex != strings.ToLower(candidate.BlockHex) || prospectjob.ValidateManifest(job) != nil || job.PublicKey != config.PublicKey || !matchesManifestPolicy(job, config) || candidate.Version != 1 || candidate.JobID != job.ID || candidate.PayoutAddress != config.PayoutAddress || candidate.Proof.JobID != job.ID || candidate.Proof.GymID != job.GymID || candidate.Proof.Version != 2 || len(candidate.BlockHex) > 2*wire.MaxBlockPayload || len(candidate.BlockHex) < 162 {
		return nil, ErrCorrupt
	}
	if _, err := gymauth.AdmissionDigest(job.Context, candidate.Proof, job.PublicKey); err != nil {
		return nil, ErrCorrupt
	}
	raw, err := hex.DecodeString(candidate.BlockHex)
	if err != nil {
		return nil, ErrCorrupt
	}
	reader := bytes.NewReader(raw)
	block := new(wire.MsgBlock)
	if block.Deserialize(reader) != nil || reader.Len() != 0 || len(block.Transactions) == 0 || len(block.Transactions) != int(candidate.TransactionCount) {
		return nil, ErrCorrupt
	}
	var encoded, header, stripped bytes.Buffer
	if block.Serialize(&encoded) != nil || !bytes.Equal(raw, encoded.Bytes()) || block.Header.Serialize(&header) != nil || hex.EncodeToString(header.Bytes()) != candidate.Proof.Header {
		return nil, ErrCorrupt
	}
	coinbase := block.Transactions[0]
	if !blockchain.IsCoinBaseTx(coinbase) || blockchain.CheckTransactionSanity(btcutil.NewTx(coinbase)) != nil || blockchain.CheckSerializedHeight(btcutil.NewTx(coinbase), int32(job.Context.ParentHeight+1)) != nil || coinbase.SerializeNoWitness(&stripped) != nil || hex.EncodeToString(stripped.Bytes()) != candidate.Proof.Coinbase || len(coinbase.TxOut) < 2 || len(coinbase.TxOut) > 3 {
		return nil, ErrCorrupt
	}
	destination, err := address.DecodeAddress(config.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil {
		return nil, ErrCorrupt
	}
	script, err := txscript.PayToAddrScript(destination)
	if err != nil {
		return nil, ErrCorrupt
	}
	commitment, err := prospectjob.ManifestCommitmentScript(job)
	if err != nil || coinbase.TxOut[0].Value != candidate.CoinbaseValue || !bytes.Equal(coinbase.TxOut[0].PkScript, script) || coinbase.TxOut[1].Value != 0 || !bytes.Equal(coinbase.TxOut[1].PkScript, commitment) || (len(coinbase.TxOut) == 3 && coinbase.TxOut[2].Value != 0) {
		return nil, ErrCorrupt
	}
	if len(coinbase.TxOut) == 3 && (len(coinbase.TxOut[2].PkScript) != blockchain.CoinbaseWitnessPkScriptLength || !bytes.HasPrefix(coinbase.TxOut[2].PkScript, blockchain.WitnessMagicBytes)) {
		return nil, ErrCorrupt
	}
	wrapped := btcutil.NewBlock(block)
	if blockchain.CalcMerkleRoot(wrapped.Transactions(), false) != block.Header.MerkleRoot || blockchain.ValidateWitnessCommitment(wrapped) != nil || blockchain.GetBlockWeight(wrapped) != candidate.Weight || candidate.Weight > blockchain.MaxBlockWeight || block.SerializeSizeStripped() > blockchain.MaxBlockBaseSize || candidate.SigOpCost < 0 || candidate.SigOpCost > blockchain.MaxBlockSigOpsCost {
		return nil, ErrCorrupt
	}
	current := coinbase.TxHash()
	for _, sibling := range candidate.Proof.MerkleBranch {
		if len(sibling) != 64 {
			return nil, ErrCorrupt
		}
		right, err := chainhash.NewHashFromStr(sibling)
		if err != nil {
			return nil, ErrCorrupt
		}
		var pair [64]byte
		copy(pair[:32], current[:])
		copy(pair[32:], right[:])
		current = chainhash.DoubleHashH(pair[:])
	}
	if current != block.Header.MerkleRoot {
		return nil, ErrCorrupt
	}
	return block, nil
}
func classifyManifest(hash chainhash.Hash, job prospectjob.ManifestJob) (bool, bool) {
	number := blockchain.HashToBig(&hash)
	target, _ := new(big.Int).SetString(job.Context.ProspectTarget, 16)
	return number.Cmp(blockchain.CompactToBig(job.Context.ExpectedBits)) <= 0, number.Cmp(target) <= 0
}
func validateManifestEntry(entry ManifestEntry, config ManifestConfig) error {
	if entry.Version != Version2 || entry.Sequence == 0 || entry.NextAttemptMS < 0 || entry.RetryCount > 32 || len(entry.WorkHash) != 64 || (!entry.BlockWin && !entry.ProspectWin) {
		return ErrCorrupt
	}
	digest, err := chainhash.NewHashFromStr(entry.WorkHash)
	if err != nil || digest.String() != entry.WorkHash {
		return ErrCorrupt
	}
	if _, err := manifestWorkBlock(entry.Job, entry.Candidate, config); err != nil {
		return ErrCorrupt
	}
	block, prospect := classifyManifest(*digest, entry.Job)
	if block != entry.BlockWin || prospect != entry.ProspectWin {
		return ErrCorrupt
	}
	switch entry.BlockState {
	case "not-applicable":
		if entry.BlockWin {
			return ErrCorrupt
		}
	case "pending", "submitted", "canonical", "rejected":
		if !entry.BlockWin {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	switch entry.AdmissionState {
	case "not-applicable":
		if entry.ProspectWin {
			return ErrCorrupt
		}
	case "needs-signature", "pending", "accepted", "expired", "rejected":
		if !entry.ProspectWin {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	if entry.Authorization != nil {
		if !manifestCanonicalAuth(*entry.Authorization) {
			return ErrCorrupt
		}
		encoded, _ := json.Marshal(entry.Authorization)
		if len(encoded) > gymauth.MaxAuthorizationSize || !entry.ProspectWin || gymauth.VerifyAuthorization(entry.Job.Context, entry.Candidate.Proof, *entry.Authorization) != nil {
			return ErrCorrupt
		}
	} else if entry.AdmissionState == "pending" || entry.AdmissionState == "accepted" {
		return ErrCorrupt
	}
	if entry.Receipt != nil {
		if entry.AdmissionState != "accepted" || manifestReceiptMatches(entry, config) != nil {
			return ErrCorrupt
		}
	} else if entry.AdmissionState == "accepted" {
		return ErrCorrupt
	}
	switch entry.LastError {
	case "", "unavailable", "outcome_unknown", "busy", "capacity", "rate_limited", "expired", "rejected", "unknown_job", "unauthorized", "forbidden", "invalid_request", "too_large", "signer_unavailable", "clock", "stale":
	default:
		return ErrCorrupt
	}
	return nil
}

// Recompute inexpensive receipt fields from the claimed digest. Opening a
// journal does not rehash history; recovered pending work is hashed before any
// transport, and stored receipts remain conditional evidence from an authority.
func expectedManifestVerified(entry ManifestEntry) (prospect.Verified, error) {
	context := entry.Job.Context
	seed, err := fighter.ParseSeed(entry.WorkHash)
	if err != nil {
		return prospect.Verified{}, ErrCorrupt
	}
	dna, err := fighter.Generate(context.DNAVersion, seed)
	if err != nil {
		return prospect.Verified{}, ErrCorrupt
	}
	genesis, _ := hex.DecodeString(context.Genesis)
	target, _ := hex.DecodeString(context.ProspectTarget)
	var integer [4]byte
	binary.LittleEndian.PutUint32(integer[:], context.Network)
	policy := append([]byte("SLUG prospect policy\x00v1\x00"), integer[:]...)
	policy = append(policy, genesis...)
	binary.LittleEndian.PutUint32(integer[:], context.DNAVersion)
	policy = append(policy, integer[:]...)
	policy = append(policy, target...)
	policyHash := sha256.Sum256(policy)
	binary.LittleEndian.PutUint32(integer[:], context.Network)
	identifier := append([]byte("SLUG prospect id\x00v1\x00"), integer[:]...)
	identifier = append(identifier, genesis...)
	identifier = append(identifier, seed[:]...)
	idHash := sha256.Sum256(identifier)
	return prospect.Verified{Version: 2, Verification: "supplied-context", Network: context.Network, Genesis: context.Genesis,
		ProspectID: hex.EncodeToString(idHash[:]), PolicyID: hex.EncodeToString(policyHash[:]), GymID: entry.Job.GymID,
		WorkHash: entry.WorkHash, ParentHash: context.ParentHash, ParentHeight: context.ParentHeight, DNA: dna}, nil
}

func manifestCanonicalProof(p prospect.Proof) bool {
	if p.Version != 2 || !manifestHashShape(p.JobID) || !manifestHashShape(p.GymID) || len(p.Header) != 160 || len(p.Coinbase) == 0 || len(p.Coinbase) > 32768 || len(p.Coinbase)%2 != 0 || len(p.MerkleBranch) > 20 || p.MerkleBranch == nil {
		return false
	}
	if p.Header != strings.ToLower(p.Header) || p.Coinbase != strings.ToLower(p.Coinbase) {
		return false
	}
	for _, h := range p.MerkleBranch {
		if !manifestHashShape(h) {
			return false
		}
	}
	return true
}
func manifestCanonicalAuth(a gymauth.Authorization) bool {
	return a.Version == 2 && len(a.PublicKey) == 66 && len(a.Signature) > 0 && len(a.Signature) <= 144 && a.PublicKey == strings.ToLower(a.PublicKey) && a.Signature == strings.ToLower(a.Signature)
}

// This is an internal consistency check, not a public proof/provenance API.
func manifestReceiptMatches(e ManifestEntry, c ManifestConfig) error {
	if e.Receipt == nil {
		return ErrCorrupt
	}
	result := e.Receipt
	r := result.Record
	if !manifestCanonicalProof(r.Proof) || !manifestCanonicalAuth(r.Authorization) || result.Version != 3 || result.Authentication != "gym-key-control" || result.BirthRule != "job-issuance" || result.Inserted != result.CurrentNodeChecked || (result.Inserted && result.ContextSource != "trusted-node-snapshot") || (!result.Inserted && result.ContextSource != "historical-receipt") || r.Version != registry.Version5 || r.Sequence == 0 || r.Job != e.Job || r.Context != e.Job.Context || r.Verified.WorkHash != e.WorkHash || r.Birth.RuleVersion != e.Job.RuleVersion || r.Birth.BornAtMS != e.Job.IssuedAtMS || r.Birth.AcceptedAtMS < e.Job.IssuedAtMS || r.Birth.AcceptedAtMS >= e.Job.ExpiresAtMS || r.Authorization.PublicKey != e.Job.PublicKey || gymauth.VerifyAuthorization(e.Job.Context, r.Proof, r.Authorization) != nil || !equalProof(r.Proof, e.Candidate.Proof) || registry.ValidateEligibility(e.Job.Context, r.Verified, r.Eligibility) != nil {
		return ErrCorrupt
	}
	expected, err := expectedManifestVerified(e)
	if err != nil || r.Verified != expected {
		return ErrCorrupt
	}
	binding, err := manifestStaticBinding(e.Job.DescriptorID, expected)
	if err != nil || r.StaticBinding != binding {
		return ErrCorrupt
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > registry.MaxRecordSize {
		return ErrCorrupt
	}
	raw, err = json.Marshal(result)
	if err != nil || len(raw) > prospectservice.MaxResponseSize {
		return ErrCorrupt
	}
	if r.Eligibility.Source == "canonical-winning-block" && !e.BlockWin {
		return ErrCorrupt
	}
	return nil
}
func manifestStaticBinding(id string, v prospect.Verified) (registry.StaticBinding, error) {
	d, err := fighterdescriptor.ResolveCandidate(id)
	if err != nil || !manifestHashShape(v.WorkHash) || d.RendererID != fighterportrait.RendererID {
		return registry.StaticBinding{}, ErrCorrupt
	}
	m, err := fightermanifest.GenerateText(d.ManifestVersion, v.WorkHash)
	if err != nil || m.DNA != v.DNA || m.DNA.DNAVersion != d.DNAVersion || m.Version != d.ManifestVersion || m.Generator != d.GeneratorID || m.Tables != d.ManifestTableHash || m.Portrait.Version != d.PortraitTraitVersion {
		return registry.StaticBinding{}, ErrCorrupt
	}
	md, err := fightermanifest.Digest(m)
	if err != nil {
		return registry.StaticBinding{}, ErrCorrupt
	}
	pd, err := fighterportrait.Digest(d.RendererVersion, m)
	if err != nil {
		return registry.StaticBinding{}, ErrCorrupt
	}
	return registry.StaticBinding{Version: 1, DescriptorID: id, ManifestDigest: md, PortraitDigest: pd}, nil
}
func manifestTemplateBounds(t prospectwork.Template) bool {
	b := t.Block
	if b.CoinbaseTxn != nil || len(b.Transactions) > 16666 || len(b.Mutable) > prospectwork.MaxTemplateSize/3 || len(b.Capabilities) > prospectwork.MaxTemplateSize/3 {
		return false
	}
	total := 3*len(b.Mutable) + 3*len(b.Capabilities) + len(b.WorkID) + len(b.DefaultWitnessCommitment) + len(b.LongPollID) + len(b.LongPollURI) + len(b.RejectReason) + len(b.Bits) + len(b.PreviousHash) + len(b.Target) + len(b.NonceRange)
	if b.CoinbaseAux != nil {
		total += len(b.CoinbaseAux.Flags)
	}
	for _, s := range b.Mutable {
		if len(s) > prospectwork.MaxTemplateSize-total {
			return false
		}
		total += len(s)
	}
	for _, s := range b.Capabilities {
		if len(s) > prospectwork.MaxTemplateSize-total {
			return false
		}
		total += len(s)
	}
	for _, tx := range b.Transactions {
		if len(tx.Depends) > len(b.Transactions) || len(tx.Data) > 2*wire.MaxBlockPayload || len(tx.TxID) != 64 || len(tx.Hash) != 64 {
			return false
		}
		total += len(tx.Data) + 128 + 8*len(tx.Depends)
		if total > prospectwork.MaxTemplateSize {
			return false
		}
	}
	if total > prospectwork.MaxTemplateSize {
		return false
	}
	raw, err := json.Marshal(b)
	return err == nil && len(raw) <= prospectwork.MaxTemplateSize
}
