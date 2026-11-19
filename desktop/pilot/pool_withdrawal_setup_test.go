// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
	"github.com/taytr-llc/slugwallet/walletdb"
	"github.com/taytr-llc/slugwallet/wtxmgr"
)

func withdrawalSetupRuntime(t *testing.T) *PoolRuntime {
	t.Helper()
	r := poolRuntimeTest(t)
	var e error
	r.config.DataDir, e = filepath.EvalSymlinks(r.config.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func withdrawalSetupWallet(t *testing.T, fixture string) (*PoolRuntime, PoolRecipientBinding, *walletRuntime) {
	t.Helper()
	r := withdrawalSetupRuntime(t)
	b := poolRuntimeTestBinding(t, r, fixture)
	w, e := openAccountFixtureWallet(filepath.Join(r.config.DataDir, "fixture-wallet"), fixture, true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := w.Close(); e != nil {
			t.Error(e)
		}
	})
	return r, b, w
}
func withdrawalSetupAccounts(t *testing.T, w *walletRuntime) []waddrmgr.RecoveryAccount {
	t.Helper()
	var out []waddrmgr.RecoveryAccount
	if e := walletdb.View(w.wallet.Database(), func(tx walletdb.ReadTx) error {
		var e error
		out, e = waddrmgr.RecoveryAccounts(tx.ReadBucket([]byte("waddrmgr")))
		return e
	}); e != nil {
		t.Fatal(e)
	}
	return out
}
func withdrawalSetupDropNamespace(t *testing.T, w *walletRuntime) {
	t.Helper()
	if e := walletdb.Update(w.wallet.Database(), func(tx walletdb.ReadWriteTx) error {
		return tx.DeleteTopLevelBucket([]byte("slug-candidate-withdrawal-v1"))
	}); e != nil {
		t.Fatal(e)
	}
}
func withdrawalSetupBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestPoolWithdrawalSetupRootInventory(t *testing.T) {
	for _, name := range []string{"directory_without_guard", "foreign_account", "scope_unknown_entry", "scope_symlink", "guard_symlink", "guard_mismatch", "directory_symlink", "scope_wrong_mode"} {
		t.Run(name, func(t *testing.T) {
			r := withdrawalSetupRuntime(t)
			b := poolRuntimeTestBinding(t, r, "zero256")
			r.gymAccounts = map[string]string{"zero256": b.Config.AccountID}
			if e := r.validateWithdrawalRoot(); e != nil {
				t.Fatal("absent initialization", e)
			}
			guard, e := r.withdrawalGuard()
			if e != nil {
				t.Fatal(e)
			}
			gp := filepath.Join(r.config.DataDir, poolWithdrawalGuardName)
			if name != "directory_without_guard" {
				if e := writePoolRuntimeRecord(gp, guard); e != nil {
					t.Fatal(e)
				}
				if e := r.validateWithdrawalRoot(); e != nil {
					t.Fatal("guard-only resume", e)
				}
			}
			dir := filepath.Join(r.config.DataDir, poolWithdrawalDirectory)
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			scope := filepath.Join(dir, b.Config.AccountID)
			if e := os.Mkdir(scope, 0700); e != nil {
				t.Fatal(e)
			}
			switch name {
			case "foreign_account":
				if e := os.Rename(scope, filepath.Join(dir, strings.Repeat("e", 64))); e != nil {
					t.Fatal(e)
				}
			case "scope_unknown_entry":
				if e := os.WriteFile(filepath.Join(scope, "unexpected"), nil, 0600); e != nil {
					t.Fatal(e)
				}
			case "scope_symlink":
				if e := os.Symlink(gp, filepath.Join(scope, "INIT.json")); e != nil {
					t.Fatal(e)
				}
			case "guard_symlink":
				raw := withdrawalSetupBytes(t, gp)
				if e := os.Remove(gp); e != nil {
					t.Fatal(e)
				}
				target := filepath.Join(r.config.DataDir, "guard-target")
				if e := os.WriteFile(target, raw, 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, gp); e != nil {
					t.Fatal(e)
				}
			case "guard_mismatch":
				guard.AccountAuthorityID = strings.Repeat("f", 64)
				raw, _ := json.Marshal(guard)
				if e := os.WriteFile(gp, append(raw, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
			case "directory_symlink":
				target := filepath.Join(r.config.DataDir, "directory-target")
				if e := os.Rename(dir, target); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, dir); e != nil {
					t.Fatal(e)
				}
			case "scope_wrong_mode":
				if e := os.Chmod(scope, 0755); e != nil {
					t.Fatal(e)
				}
			}
			if e := r.validateWithdrawalRoot(); e == nil {
				t.Fatal("invalid root inventory accepted")
			}
		})
	}
}
func TestPoolWithdrawalSetupEmptyResumeDoesNotAllocate(t *testing.T) {
	for _, fixture := range []string{"zero256", "incrementing256"} {
		t.Run(fixture, func(t *testing.T) {
			r, b, w := withdrawalSetupWallet(t, fixture)
			before := withdrawalSetupAccounts(t, w)
			first, e := r.ensureWithdrawalScope(context.Background(), b, w)
			if e != nil {
				t.Fatal(e)
			}
			if e := first.check(); e != nil {
				t.Fatal(e)
			}
			root := filepath.Dir(first.journalPath)
			ip, rp := filepath.Join(root, "INIT.json"), filepath.Join(root, "READY.json")
			ib, rb := withdrawalSetupBytes(t, ip), withdrawalSetupBytes(t, rp)
			for _, removeWallet := range []bool{false, true} {
				if e := os.Remove(rp); e != nil {
					t.Fatal(e)
				}
				if removeWallet {
					withdrawalSetupDropNamespace(t, w)
				}
				resumed, e := r.ensureWithdrawalScope(context.Background(), b, w)
				if e != nil {
					t.Fatal("original empty resume", removeWallet, e)
				}
				if resumed.Identity != first.Identity || resumed.BindingID != first.BindingID || !bytes.Equal(ib, withdrawalSetupBytes(t, ip)) || !bytes.Equal(rb, withdrawalSetupBytes(t, rp)) {
					t.Fatal("resume replaced original identity/markers")
				}
			}
			again, e := r.ensureWithdrawalScope(context.Background(), b, w)
			if e != nil || again.Identity != first.Identity {
				t.Fatal("established reopen", e)
			}
			if !reflect.DeepEqual(before, withdrawalSetupAccounts(t, w)) {
				t.Fatal("namespace setup allocated an address")
			}
		})
	}
}
func TestPoolWithdrawalSetupEstablishedLossNeverRecreates(t *testing.T) {
	r, b, w := withdrawalSetupWallet(t, "zero256")
	scope, e := r.ensureWithdrawalScope(context.Background(), b, w)
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Dir(scope.journalPath)
	before := withdrawalSetupAccounts(t, w)
	paths := []string{filepath.Join(r.config.DataDir, poolWithdrawalGuardName), filepath.Join(root, "INIT.json"), filepath.Join(scope.journalPath, "SLUG-WITHDRAWAL-JOURNAL.json"), filepath.Join(scope.journalPath, "state.json")}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw := withdrawalSetupBytes(t, path)
			if e := os.Remove(path); e != nil {
				t.Fatal(e)
			}
			defer func() {
				if e := os.WriteFile(path, raw, 0600); e != nil {
					t.Error(e)
				}
			}()
			if _, e := r.ensureWithdrawalScope(context.Background(), b, w); e == nil {
				t.Fatal("missing established component recreated")
			}
			if _, e := os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("refusal mutated missing component", e)
			}
		})
	}
	t.Run("wallet_namespace", func(t *testing.T) {
		withdrawalSetupDropNamespace(t, w)
		if _, e := r.ensureWithdrawalScope(context.Background(), b, w); e == nil {
			t.Fatal("missing established namespace recreated")
		}
		if s, e := w.wallet.OpenCandidateWithdrawals(context.Background(), scope.BindingID); !errors.Is(e, wallet.ErrWithdrawalNotFound) {
			if s != nil {
				s.Close()
			}
			t.Fatal("missing namespace repaired", e)
		}
		s, e := w.wallet.CreateCandidateWithdrawals(context.Background(), scope.BindingID)
		if e != nil {
			t.Fatal(e)
		}
		if e := s.Close(); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("changed_ready", func(t *testing.T) {
		path := filepath.Join(root, "READY.json")
		raw := withdrawalSetupBytes(t, path)
		changed := scope.ready
		changed.JournalID = strings.Repeat("f", 64)
		bad, _ := json.Marshal(changed)
		bad = append(bad, '\n')
		if e := os.WriteFile(path, bad, 0600); e != nil {
			t.Fatal(e)
		}
		defer func() {
			if e := os.WriteFile(path, raw, 0600); e != nil {
				t.Error(e)
			}
		}()
		if _, e := r.ensureWithdrawalScope(context.Background(), b, w); e == nil {
			t.Fatal("mismatched ready adopted")
		}
		if !bytes.Equal(bad, withdrawalSetupBytes(t, path)) {
			t.Fatal("mismatched ready rewritten")
		}
	})
	if !reflect.DeepEqual(before, withdrawalSetupAccounts(t, w)) {
		t.Fatal("failed setup allocated an address")
	}
}
func TestPoolWithdrawalSetupWalletWithoutJournalRefuses(t *testing.T) {
	r, b, w := withdrawalSetupWallet(t, "zero256")
	scope, e := r.ensureWithdrawalScope(context.Background(), b, w)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(filepath.Join(filepath.Dir(scope.journalPath), "READY.json")); e != nil {
		t.Fatal(e)
	}
	if e := os.RemoveAll(scope.journalPath); e != nil {
		t.Fatal(e)
	}
	if _, e := r.ensureWithdrawalScope(context.Background(), b, w); e == nil {
		t.Fatal("orphan wallet namespace adopted")
	}
	if _, e := os.Lstat(scope.journalPath); !os.IsNotExist(e) {
		t.Fatal("orphan namespace recreated app journal", e)
	}
}

// The exact original public Set61 payout supplies wallet storage input; no node
// validity or new transaction acceptance is inferred from this local fixture.
func withdrawalSetupFund(t *testing.T, w *walletRuntime) {
	t.Helper()
	raw := withdrawalSetupBytes(t, "../../../slugwallet/wallet/testdata/candidate-withdrawals/reference-inputs/public/original-set61-payout.json")
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "d1808376a6f54b8d5816f33c390f3314e91f38013db83583683657370a0749da" {
		t.Fatal("original public input changed")
	}
	var saved struct {
		Batches []struct {
			Raw string `json:"raw"`
		} `json:"batches"`
	}
	if json.Unmarshal(raw, &saved) != nil || len(saved.Batches) != 1 {
		t.Fatal("original input shape")
	}
	txraw, _ := hex.DecodeString(saved.Batches[0].Raw)
	var tx wire.MsgTx
	if e := tx.Deserialize(bytes.NewReader(txraw)); e != nil {
		t.Fatal(e)
	}
	rec, e := wtxmgr.NewTxRecordFromMsgTx(&tx, time.Unix(1, 0))
	if e != nil {
		t.Fatal(e)
	}
	if rec.Hash.String() != "eb3bd019318cb8ce5635794caae4b970fbb0480f905ea61e1033289a9ab3ed26" {
		t.Fatal("original txid mismatch")
	}
	tip, _ := chainhash.NewHashFromStr("484db6919991b669066251626d502fb2b304559429e648da1db993c812221795")
	e = walletdb.Update(w.wallet.Database(), func(db walletdb.ReadWriteTx) error {
		ns := db.ReadWriteBucket([]byte("waddrmgr"))
		m, e := w.wallet.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeBIP0084)
		if e != nil {
			return e
		}
		if _, e = m.NextExternalAddresses(ns, 0, 1); e != nil {
			return e
		}
		tn := db.ReadWriteBucket([]byte("wtxmgr"))
		block := wtxmgr.BlockMeta{Block: wtxmgr.Block{Hash: *tip, Height: 109}, Time: time.Unix(1, 0)}
		if e = w.wallet.TxStore.InsertTx(tn, rec, &block); e != nil {
			return e
		}
		if e = w.wallet.TxStore.AddCredit(tn, rec, &block, 1, false); e != nil {
			return e
		}
		return w.wallet.Manager.SetSyncedTo(ns, &waddrmgr.BlockStamp{Hash: *tip, Height: 109, Timestamp: time.Unix(1, 0)})
	})
	if e != nil {
		t.Fatal(e)
	}
	w.wallet.SetChainSynced(true)
}
func TestPoolWithdrawalSetupPopulatedBeforeReadyRefuses(t *testing.T) {
	for _, side := range []string{"wallet", "app"} {
		t.Run(side, func(t *testing.T) {
			r, b, w := withdrawalSetupWallet(t, "zero256")
			scope, e := r.ensureWithdrawalScope(context.Background(), b, w)
			if e != nil {
				t.Fatal(e)
			}
			request := walletwithdraw.Request{Version: 1, OperationID: strings.Repeat("1", 64), Recipient: poolRecipientOracle(t, "incrementing256").Payout.Address, AmountAtoms: 100000000}
			var savedDraft wallet.CandidateWithdrawalDraft
			if side == "wallet" {
				withdrawalSetupFund(t, w)
				s, e := w.wallet.OpenCandidateWithdrawals(context.Background(), scope.BindingID)
				if e != nil {
					t.Fatal(e)
				}
				savedDraft, e = s.Prepare(context.Background(), wallet.CandidateWithdrawalRequest{Version: request.Version, OperationID: request.OperationID, Recipient: request.Recipient, AmountAtoms: request.AmountAtoms})
				closeErr := s.Close()
				if e != nil || closeErr != nil {
					t.Fatal(e, closeErr)
				}
			} else {
				j, e := walletwithdraw.Open(context.Background(), scope.journalPath, scope.Identity)
				if e != nil {
					t.Fatal(e)
				}
				o, e := j.TryAcquire(context.Background())
				if e != nil {
					j.Close()
					t.Fatal(e)
				}
				p, e := o.StageIntent(request)
				if e == nil {
					_, e = p.Publish(func(fn func(func() error) error) error { return fn(func() error { return nil }) })
				}
				e = errors.Join(e, o.Close(), j.Close())
				if e != nil {
					t.Fatal(e)
				}
			}
			before := withdrawalSetupAccounts(t, w)
			rp := filepath.Join(filepath.Dir(scope.journalPath), "READY.json")
			if e := os.Remove(rp); e != nil {
				t.Fatal(e)
			}
			if _, e := r.ensureWithdrawalScope(context.Background(), b, w); e == nil {
				t.Fatal("populated pre-ready component adopted")
			}
			if _, e := os.Lstat(rp); !os.IsNotExist(e) {
				t.Fatal("populated state published ready", e)
			}
			if !reflect.DeepEqual(before, withdrawalSetupAccounts(t, w)) {
				t.Fatal("refusal changed address cursors")
			}
			if side == "wallet" {
				s, e := w.wallet.OpenCandidateWithdrawals(context.Background(), scope.BindingID)
				if e != nil {
					t.Fatal(e)
				}
				record, e := s.Lookup(context.Background(), request.OperationID)
				closeErr := s.Close()
				if e != nil || closeErr != nil || record.State != "prepared" || !reflect.DeepEqual(record.Draft, savedDraft) {
					t.Fatal("refusal changed prepared wallet record", e, closeErr)
				}
			} else {
				j, e := walletwithdraw.Open(context.Background(), scope.journalPath, scope.Identity)
				if e != nil {
					t.Fatal(e)
				}
				o, e := j.TryAcquire(context.Background())
				if e != nil {
					j.Close()
					t.Fatal(e)
				}
				record, e := o.Lookup(request.OperationID)
				e = errors.Join(e, o.Close(), j.Close())
				if e != nil || record.Stage != walletwithdraw.StageIntent || record.Intent.Destination != request.Recipient || record.Intent.RecipientAtoms != request.AmountAtoms {
					t.Fatal("refusal changed app intent", e)
				}
			}
		})
	}
}
