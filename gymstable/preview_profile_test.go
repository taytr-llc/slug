//go:build slugtestpow

package gymstable

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPreviewCheapProfileRefusesPair(t *testing.T) {
	s := &Store{}
	r, e := s.ReadPreviewPair(context.Background(), Session{}, [2]string{strings.Repeat("1", 64), strings.Repeat("2", 64)})
	if !errors.Is(e, ErrProfile) || r.Version != 0 {
		t.Fatal("profile refusal before handle access", r, e)
	}
}
