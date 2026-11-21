// Closed original-owner admission evidence. Never inspect live or private DBs.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
)

type retainedAdmissionCounts struct {
	Records          uint64 `json:"records"`
	Sequences        uint64 `json:"sequences"`
	OwnershipOutbox  uint64 `json:"ownership_outbox"`
	SignedAdmissions uint64 `json:"signed_admissions"`
	Publications     uint64 `json:"publications"`
}

type retainedAdmissionAudit struct {
	Financial              attachmentOperatorAudit            `json:"financial"`
	TargetProspectID       string                             `json:"target_prospect_id"`
	JobID                  string                             `json:"job_id"`
	Job                    prospectjob.ManifestJob            `json:"job"`
	Proof                  prospect.Proof                     `json:"proof"`
	Authorization          gymauth.Authorization              `json:"authorization"`
	Record                 *registry.ManifestRecord           `json:"record"`
	RecordSHA256           string                             `json:"record_sha256"`
	RecordBytes            int                                `json:"record_bytes"`
	TargetRecords          uint64                             `json:"target_records"`
	TargetSequences        uint64                             `json:"target_sequences"`
	TargetOutbox           uint64                             `json:"target_outbox"`
	TargetSignedAdmissions uint64                             `json:"target_signed_admissions"`
	TargetPublications     uint64                             `json:"target_publications"`
	OwnershipState         string                             `json:"ownership_state"`
	Admission              *registry.PublicAdmissionStatement `json:"admission"`
	AdmissionID            string                             `json:"admission_id"`
	Publication            *registry.PublicPublicationEntry   `json:"publication"`
	Counts                 retainedAdmissionCounts            `json:"counts"`
	IssuerRows             map[string]string                  `json:"issuer_rows"`
	PublicFilesBefore      map[string]string                  `json:"public_files_before"`
	PublicFilesAfter       map[string]string                  `json:"public_files_after"`
	ReadOnlyUnchanged      bool                               `json:"read_only_unchanged"`
}

// Caller has joined every operator, CLI and relay handler for these exact roots.
// A/B decide whether the target must exist. This routine permits unrelated
// prospects and proves the target independently of aggregate birth counts.
func auditRetainedAdmission(ctx context.Context, root string, selected pooloperator.RecoverableDailySetup, proof prospect.Proof, authorization gymauth.Authorization) (out retainedAdmissionAudit) {
	require(ctx != nil, "original retained admission audit context")
	check(ctx.Err(), "retained admission audit context")
	before := attachmentPublicFiles(root)
	out.PublicFilesBefore = before
	out.Financial = auditOperator(ctx, root, selected)
	require(out.Financial.ReadOnlyUnchanged && out.Financial.Accepted == 2 && out.Financial.Submissions == 2 && len(out.Financial.Entries) == 2, "complete original two-receipt source remains accepted")
	var owners auditOwnerStack
	defer owners.finish()
	profile, e := registry.NewOperatorProfile(selected.Base.Authority)
	check(e, "retained admission original operator profile")
	codec, e := profile.Codec()
	check(e, "original public evidence codec")
	issuer, e := registry.OpenRecoverableOperatorManifestAuthorityAudit(ctx, filepath.Join(root, "operator", "issuer"), profile, selected.IssueProfile)
	if issuer != nil {
		owners.opened("retained admission original issuer", issuer.Close, e)
	}
	check(e, "full original selected issuer audit")
	info, requests, e := issuer.AuditIssueBinding(ctx, selected.Base.Pool.AuthorityPin, selected.IssueProfile)
	check(e, "complete original issue census")
	require(info.ReadOnly && info.Pin == selected.Base.Pool.AuthorityPin && info.Records == out.Financial.Births && info.Jobs == out.Financial.AuthorityJobs && requests == out.Financial.IssuerRequests, "unchanged full issuer census")
	out.Job, e = issuer.TryGetManifestJob(ctx, proof.JobID)
	check(e, "retained target original issued job")
	require(proof.Version == 2 && authorization.Version == 2 && proof.JobID == out.Job.ID && proof.GymID == out.Job.GymID && authorization.PublicKey == out.Job.PublicKey, "exact original target job gym and key")
	check(gymauth.VerifyAuthorization(out.Job.Context, proof, authorization), "original target authorization")
	verified, e := prospect.Verify(out.Job.Context, proof)
	check(e, "original normal-PoW target verification")
	out.TargetProspectID, out.JobID = verified.ProspectID, out.Job.ID
	out.Proof = proof
	out.Proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	out.Authorization = authorization
	first, second := out.Financial.Entries[0], out.Financial.Entries[1]
	require(first.Share.AcceptedSequence == 1 && first.Share.Block && first.RewardAtoms > 0 && second.Share.AcceptedSequence == 2 && second.Share.Block && second.RewardAtoms > 0, "two original accepted winning receipts")
	require(second.Job == out.Job && second.Proof != nil && reflect.DeepEqual(*second.Proof, out.Proof) && second.Share.JobID == out.JobID && second.Share.WorkHash == verified.WorkHash, "target is exact original second accepted pool winner")
	record, e := issuer.GetManifestAdmission(out.TargetProspectID)
	if e == nil {
		require(record.Job == out.Job && record.Verified == verified && reflect.DeepEqual(record.Proof, out.Proof) && record.Authorization == authorization, "exact original target record proof authorization and job")
		require(record.Birth.BornAtMS == out.Job.IssuedAtMS && record.Birth.AcceptedAtMS >= out.Job.IssuedAtMS && record.Birth.AcceptedAtMS < out.Job.ExpiresAtMS, "target retains original live acceptance and issuance birth")
		out.Record = &record
	} else {
		require(errors.Is(e, registry.ErrNotFound), "only exact missing target is absence")
	}
	check(owners.close(), "original issuer audit joined before row projection")

	// Full original audits above authenticate the namespaces. This separate
	// bounded StrictAll pass correlates exact bytes/counts without cached state.
	expectedRows := map[string]string{}
	for k, v := range out.Financial.PublicRows {
		if strings.HasPrefix(k, "issuer/") {
			expectedRows[k] = v
		}
	}
	var meta struct {
		Accepted     uint32 `json:"accepted_sequence"`
		Signed       uint32 `json:"signed_admission_sequence"`
		Publications uint32 `json:"publication_sequence"`
	}
	metaSeen := false
	targetSequence := uint64(0)
	if out.Record != nil {
		targetSequence = out.Record.Sequence
	}
	refs := map[uint32][]registry.PublicPublicationEntry{}
	rowSeq := func(k []byte, prefix string) uint32 {
		require(len(k) == len(prefix)+4, "exact ownership counter key")
		n := binary.BigEndian.Uint32(k[len(prefix):])
		require(n > 0, "positive ownership counter")
		return n
	}
	out.IssuerRows = walkAuditRows(ctx, root, "issuer", func(k, v []byte) {
		require(expectedRows["issuer/"+hx(k)] == hx(digest(v)), "same fully audited issuer bytes")
		switch {
		case bytes.Equal(k, []byte("ownership/meta")):
			require(!metaSeen && len(v) <= registry.MaxPublicSmallSize, "one bounded public metadata")
			check(json.Unmarshal(v, &meta), "public count projection")
			metaSeen = true
		case bytes.HasPrefix(k, []byte("record:")):
			require(len(k) == len("record:")+64 && len(v) <= registry.MaxRecordSize, "bounded original record row")
			out.Counts.Records++
			if string(k[len("record:"):]) == out.TargetProspectID {
				out.TargetRecords++
				require(out.Record != nil, "unreported target record")
				raw, e := json.Marshal(out.Record)
				check(e, "exact target record encoding")
				require(bytes.Equal(raw, v), "exact original stored target record bytes")
				out.RecordSHA256, out.RecordBytes = hx(digest(v)), len(v)
			}
		case bytes.HasPrefix(k, []byte("sequence:")):
			require(len(k) == len("sequence:")+8 && len(v) == 64, "bounded original receipt index")
			out.Counts.Sequences++
			if string(v) == out.TargetProspectID {
				out.TargetSequences++
				require(out.Record != nil && binary.BigEndian.Uint64(k[len("sequence:"):]) == targetSequence, "exact target receipt reverse index")
			}
		case bytes.HasPrefix(k, []byte("ownership/outbox/")):
			n := rowSeq(k, "ownership/outbox/")
			var row struct {
				Version    uint32 `json:"version"`
				Sequence   uint32 `json:"admission_sequence"`
				ProspectID string `json:"prospect_id"`
			}
			require(len(v) <= registry.MaxPublicSmallSize, "bounded ownership outbox")
			check(json.Unmarshal(v, &row), "ownership outbox projection")
			require(row.Version == 1 && row.Sequence == n, "outbox original counter")
			out.Counts.OwnershipOutbox++
			if row.ProspectID == out.TargetProspectID {
				out.TargetOutbox++
				require(out.Record != nil && uint64(n) == targetSequence, "target outbox same receipt")
			}
		case bytes.HasPrefix(k, []byte("ownership/admission/")):
			n := rowSeq(k, "ownership/admission/")
			a, e := codec.ParseAdmission(v)
			check(e, "original signed ownership admission")
			require(a.AdmissionSequence == n, "signed admission original index")
			out.Counts.SignedAdmissions++
			if a.Record.Verified.ProspectID == out.TargetProspectID {
				out.TargetSignedAdmissions++
				require(out.Record != nil && uint64(n) == targetSequence && reflect.DeepEqual(a.Record, *out.Record), "target signed admission contains exact original record")
				out.Admission = &a
				out.AdmissionID, e = codec.AdmissionID(a)
				check(e, "target signed admission identity")
			}
		case bytes.HasPrefix(k, []byte("ownership/publication/")):
			n := rowSeq(k, "ownership/publication/")
			p, e := codec.ParsePublication(v)
			check(e, "original signed publication")
			require(p.Sequence == n, "publication original index")
			out.Counts.Publications++
			if p.Kind == "admission" {
				var ref registry.PublicAdmissionReference
				check(json.Unmarshal(p.Payload, &ref), "signed admission publication reference")
				refs[ref.AdmissionSequence] = append(refs[ref.AdmissionSequence], p)
			}
		}
	})
	require(reflect.DeepEqual(out.IssuerRows, expectedRows), "complete identical audited issuer namespace")
	require(metaSeen && uint64(meta.Accepted) == info.Records && meta.Signed <= meta.Accepted, "original public counts agree with full authority")
	// ownership/outbox is a permanent admission journal, not an unprocessed
	// queue: the original publisher retains its row after signing/publication.
	require(out.Counts.Records == info.Records && out.Counts.Sequences == info.Records && out.Counts.OwnershipOutbox == uint64(meta.Accepted) && out.Counts.SignedAdmissions == uint64(meta.Signed) && out.Counts.Publications == uint64(meta.Publications), "complete record index outbox admission publication census")
	if out.Record == nil {
		require(out.TargetRecords == 0 && out.TargetSequences == 0 && out.TargetOutbox == 0 && out.TargetSignedAdmissions == 0, "absent target has no record index or ownership evidence")
		out.OwnershipState = "absent"
	} else {
		require(out.TargetRecords == 1 && out.TargetSequences == 1 && out.TargetOutbox <= 1 && out.TargetSignedAdmissions <= 1, "exactly one original target receipt")
		if out.Admission == nil {
			require(out.TargetOutbox == 1 && len(refs[uint32(targetSequence)]) == 0, "unsigned target retains original publication outbox")
			out.OwnershipState = "pending-publication"
		} else {
			require(out.TargetOutbox == 1 && len(refs[uint32(targetSequence)]) == 1, "published target retains one journal row and one publication")
			p := refs[uint32(targetSequence)][0]
			var ref registry.PublicAdmissionReference
			check(json.Unmarshal(p.Payload, &ref), "target publication reference")
			require(ref.AdmissionID == out.AdmissionID, "target publication names exact signed admission")
			out.Publication = &p
			out.TargetPublications = 1
			out.OwnershipState = "published"
		}
	}
	// A publication of the absent target is impossible under the full original
	// signed-chain audit: every admission reference must bind an existing exact
	// signed admission/record. No caller-supplied missing sequence is substituted.
	check(ctx.Err(), "retained audit final original context")
	after := attachmentPublicFiles(root)
	require(reflect.DeepEqual(before, after), "retained admission audit changed public files")
	out.PublicFilesAfter = after
	out.ReadOnlyUnchanged = true
	return out
}
