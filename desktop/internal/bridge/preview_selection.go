// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import "encoding/json"

const legacyPreviewFamily = "preview"
const careerPreviewFamily = "career_preview"

type previewSelection struct{ Family, JobID string }

// Called only after an acknowledged, correlated Begin. A refused or lost ACK
// cannot select a replacement. Both families share this single presentation.
func (b *StableBridge) selectPreview(family, id string) {
	for key, e := range b.previewJobs {
		if stableTerminal(e.state) {
			delete(b.previewJobs, key)
		}
	}
	for key, e := range b.careerPreviewJobs {
		if stableTerminal(e.state) {
			delete(b.careerPreviewJobs, key)
		}
	}
	b.previewSnapshotJobs = nil
	b.careerPreviewSnapshotJobs = nil
	b.mu.Lock()
	b.presentation = nil
	b.previewSelection = &previewSelection{family, id}
	b.mu.Unlock()
}
func (b *StableBridge) canTrackPreview() bool {
	if b.previewCorrelationCount() < 3 {
		return true
	}
	for _, e := range b.previewJobs {
		if stableTerminal(e.state) {
			return true
		}
	}
	for _, e := range b.careerPreviewJobs {
		if stableTerminal(e.state) {
			return true
		}
	}
	return false
}

// Exact owned-job recovery may select only when no later explicit selection
// exists. Old terminal polls still correlate, but never replace that selection.
func (b *StableBridge) previewMayPresentLocked(family, id string) bool {
	if b.previewSelection == nil {
		b.previewSelection = &previewSelection{family, id}
	}
	return *b.previewSelection == (previewSelection{family, id})
}
func (b *StableBridge) clearCareerPreviewPresentation() {
	b.mu.Lock()
	if b.presentation != nil && b.presentation.metadata.Family == careerPreviewFamily {
		b.presentation = nil
	}
	b.mu.Unlock()
}

// Compact public correlations plus current authority are charged together;
// neither family retains a transcript. SVGs have separate two-by-32KiB bounds.
func (b *StableBridge) previewMetadataBound() bool {
	if b.previewCorrelationCount() > 3 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var metadata *previewPresentationMetadata
	if b.presentation != nil {
		metadata = &b.presentation.metadata
	}
	legacy := make(map[string][7]string, len(b.previewJobs))
	for id, e := range b.previewJobs {
		legacy[id] = [7]string{e.a, e.b, e.state, e.seed, e.inputID, e.digest, e.capturedResultDigest}
	}
	career := make(map[string]any, len(b.careerPreviewJobs))
	for id, e := range b.careerPreviewJobs {
		career[id] = struct {
			Tuple           careerPreviewJobExpectation
			State, Terminal string
		}{e, e.state, e.terminalDigest}
	}
	raw, err := json.Marshal(struct {
		Legacy       any
		Career       any
		LegacyIDs    any
		CareerIDs    any
		Selection    any
		Presentation any
		View         any
		StoreID      string
		Activation   any
	}{legacy, career, b.previewSnapshotJobs, b.careerPreviewSnapshotJobs, b.previewSelection, metadata, b.view, b.storeID, b.careerActivation})
	return err == nil && len(raw)+4096 <= 24576
}

// Snapshot discovery and remembered tuples may refer to the same job; count
// their union, namespaced by family, instead of charging it twice or omitting it.
func (b *StableBridge) previewCorrelationCount() int {
	ids := make(map[previewSelection]bool, 6)
	for id := range b.previewJobs {
		ids[previewSelection{legacyPreviewFamily, id}] = true
	}
	for id := range b.previewSnapshotJobs {
		ids[previewSelection{legacyPreviewFamily, id}] = true
	}
	for id := range b.careerPreviewJobs {
		ids[previewSelection{careerPreviewFamily, id}] = true
	}
	for id := range b.careerPreviewSnapshotJobs {
		ids[previewSelection{careerPreviewFamily, id}] = true
	}
	return len(ids)
}
