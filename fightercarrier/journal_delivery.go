// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import "encoding/hex"

// Delivery can only be minted by publishing an exact dispatch attempt. Closing
// its exclusive owner destroys it. Parsed durable data cannot restore it.
type Delivery struct {
	owner          *Owner
	requestID, leg string
	attempt        uint32
	raw, txid      string
	taken, issued  bool
}

func (o *Owner) TakeDelivery(id, leg string) (*Delivery, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	d := o.delivery
	if d == nil || d.requestID != id || d.leg != leg || d.issued {
		return nil, ErrConflict
	}
	d.issued = true
	return d, nil
}
func (d *Delivery) Take() ([]byte, string, uint32, error) {
	if d == nil || d.owner == nil || !d.issued || d.taken {
		return nil, "", 0, ErrConflict
	}
	if err := d.owner.check(); err != nil {
		return nil, "", 0, err
	}
	if d.owner.delivery != d {
		return nil, "", 0, ErrConflict
	}
	d.taken = true
	raw, err := hex.DecodeString(d.raw)
	if err != nil {
		return nil, "", 0, ErrCorrupt
	}
	return raw, d.txid, d.attempt, nil
}
func (j *Journal) OwnsDelivery(d *Delivery) bool {
	return j != nil && d != nil && d.owner != nil && d.owner.j == j && d.owner.check() == nil && d.issued && d.owner.delivery == d
}

// Reconciliation is the exclusive journal owner's exact retained state. It is
// used to move wallet exclusions to retained/complete or release only an
// unsigned abandonment. A caller-built operation cannot manufacture this token.
type Reconciliation struct {
	owner                                             *Owner
	id, digest, signedDigest, commitID, spendID, kind string
}

func (o *Owner) Reconciliation(id string) (*Reconciliation, error) {
	v, err := o.Lookup(id)
	if err != nil {
		return nil, err
	}
	if v.Preparation == nil {
		return nil, ErrConflict
	}
	r := &Reconciliation{owner: o, id: id, digest: v.PreparationDigest}
	if v.SignedPackage != nil {
		r.kind = "retained"
		if v.Stage == "complete" {
			r.kind = "complete"
		}
		r.signedDigest = v.SignedPackageDigest
		r.commitID = v.SignedPackage.Commit.TxID
		r.spendID = v.SignedPackage.Spend.TxID
	} else if v.Stage == "abandoned" {
		r.kind = "abandoned"
	} else {
		return nil, ErrConflict
	}
	return r, nil
}
func (r *Reconciliation) Check() error {
	if r == nil || r.owner == nil {
		return ErrConflict
	}
	v, err := r.owner.Lookup(r.id)
	if err != nil {
		return err
	}
	if v.PreparationDigest != r.digest {
		return ErrConflict
	}
	if r.kind == "abandoned" {
		if v.Stage == "abandoned" && v.SignedPackage == nil {
			return nil
		}
		return ErrConflict
	}
	if v.SignedPackage == nil || v.SignedPackageDigest != r.signedDigest || v.SignedPackage.Commit.TxID != r.commitID || v.SignedPackage.Spend.TxID != r.spendID {
		return ErrConflict
	}
	if r.kind == "complete" && v.Stage != "complete" {
		return ErrConflict
	}
	return nil
}
func (r *Reconciliation) Kind() string {
	if r == nil {
		return ""
	}
	return r.kind
}
func (r *Reconciliation) RequestID() string {
	if r == nil {
		return ""
	}
	return r.id
}
func (r *Reconciliation) PreparationDigest() string {
	if r == nil {
		return ""
	}
	return r.digest
}
func (r *Reconciliation) SignedPackageDigest() string {
	if r == nil {
		return ""
	}
	return r.signedDigest
}
func (r *Reconciliation) CommitTxID() string {
	if r == nil {
		return ""
	}
	return r.commitID
}
func (r *Reconciliation) SpendTxID() string {
	if r == nil {
		return ""
	}
	return r.spendID
}
func (j *Journal) OwnsReconciliation(r *Reconciliation) bool {
	return j != nil && r != nil && r.owner != nil && r.owner.j == j && r.Check() == nil
}
