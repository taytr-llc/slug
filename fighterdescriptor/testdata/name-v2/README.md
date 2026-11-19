# Candidate name content for Set65 — not frozen

Prepared November 19, 2026 from general knowledge at root's request. This
directory is outside the repository. It contains no production implementation,
approved generator contract, descriptor ID, new expected fighter vectors,
runtime selection or claim of production activation.

`candidate_names.json` contains the proposed **512 first-name components and
512 surnames**. The entries were manually written from general knowledge;
there was no web search, scraping, copied roster, third-party name dataset,
external image/person input, synthesized spelling or repeated-row padding.
These are ordinary name components, not a set of named characters or real
people. No portrait or physical trait is associated with a name.

The larger authored drafts are retained in `first-names-draft.txt` and
`surnames-draft.txt`, including entries not selected. `selection_provenance.json`
records explicit exclusions and the narrowing procedure. After content
exclusions, evenly spaced positions in the alphabetically sorted drafts
provide a bounded list spanning the alphabet rather than truncating at an
early letter. That preparation rule has no relationship to future fighter
seeds or runtime name draws. The complete resulting list was then read for
content; three additional surname substitutions are recorded. It is not a
demographic sample or a frequency-weighted language model.

All final components use 1–32 ASCII bytes, letters with only internal
apostrophes/hyphens permitted. Existing spellings are selected as written;
there is no runtime trimming, capitalization, transliteration or normalization.
Entries are distinct both exactly and under ASCII case-folding. The final
candidate is alphabetically ordered only for review convenience. Root must
review and choose the canonical order/encoding before any versioned contract
or independent vectors are frozen.

Run `python3 /Users/taytrrip/slug/set65-prep/audit_names.py` for the bounded
content-format audit. It checks all **262,144** component pairs, not a sample:
no two different pairs produce the same exact or case-folded display string.
Each display is first + one ASCII space + surname; components contain no
spaces, so their boundary is unambiguous. This does not mean names are unique
across fighters. Different seeds will reuse the finite name pool under D51.
The report also lists shared first/surname components, so repeated-component
names such as `Ali Ali` are visible rather than concealed by a reroll.

## Content review and its limits

The final 1,024 components were reviewed as ordinary personal names. Known
slurs, explicit sexual/derogatory components, contrived jokes, recognizable
product names and deliberately selected public-figure identities were not
used as the selection basis. Obvious commercial associations noticed in the
draft, including Bosch, Bose, Bentley, Forbes, Macdonald/Mcdonald, Abbott,
Baxter and Browning, were excluded or replaced. Some draft casing and
transliteration uncertainties were omitted instead of silently corrected.
The exclusion notes describe this candidate review, not a finding that the
underlying personal names are improper.

This is **not trademark clearance, legal research or exhaustive multilingual
moderation**. Ordinary names can also be company names, brands or words in
another language. Common components inevitably recombine into real full
names: `Hugh Grant`, `Ray Charles` and `William Wallace` are examples noticed
in the candidate product. They are accidental ordinary-name combinations,
not references, portraits, endorsements or biographical claims. The audit
records these examples. Excluding every real-person coincidence would require
a different policy and would not be solved by an informal celebrity blacklist.
No such blacklist, dynamic reroll or identity rule is proposed.

The lists include names from several naming traditions, with many established
ASCII spellings and some transliterations. They do not represent any asserted
population proportions, ethnicity, nationality or naming convention. Western
first/surname composition and loss of accents are existing engineering limits;
this content preparation does not claim culturally complete coverage. Similar
spellings remain distinct, for example Dmitri/Dimitri and Victor/Viktor; exact
collision counts do not measure spoken or visual confusion. Bring those limits,
content feedback and measured collision prevalence to the final D74 review.

## Preservation and handoff

The audit checks the existing v1 table against its already-recorded SHA256.
It never writes that file. No old names, table order, manifest, receipt,
portrait, source root or expected vector has been changed. New candidate
hashes are deliberately not presented as frozen or approved artifacts.

These files are content preparation only. After root review, freeze the exact
candidate content and contract separately, independently derive expectations,
then implement explicit opt-in v2 selection under `set65-name-diversity-plan.md`.
Current defaults remain v1; existing Pool Pilot roots must never be repinned.
No collision-rate acceptance threshold or launch population is adopted here.
