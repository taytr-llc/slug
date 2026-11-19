# Set65 independent derivation results

Root approved the exact candidate content and byte rules on November 19, 2026. The frozen bundle is recorded in `approval.json`; its relative contract/table/inherited paths remain normative. Earlier proposed documents/bindings remain preserved under `reviewed-proposals/`. Only introductory status text changed before the final contract hashes were frozen. No repository code or old expected bytes were changed.

The script, public input-selection rules, contracts, approval and archived Python inputs were SHA256-locked before outcomes. Generation and complete regeneration both passed on their first retained attempt. The 50-case v1 manifest and 50-case v1 SVG corpora were independently regenerated before the v2 corpus.

## Frozen derived identity

- Descriptor ID: `cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75`
- Manifest contract SHA256: `0283415807d33b362aaeffbb1570504085cd975bf9f3adc0b497d0c8b79856a7`
- Renderer contract SHA256: `6044ce174e9a7a466783985e34b02b34ae2785719ca77b1268f7e6ad3c717677`
- Name table domain hash: `b537490e1e602e27de01975aeec36f5fb53ba9a3a35f98143e8ecb7f9e2687c2`
- Input lock SHA256: `bae736a305e6e9019db0396740c7f57c9b66ced1f484c53863d23ddbd3b88b43`
- Reference corpus SHA256: `62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b`
- Collision report SHA256: `18b5dcc0e344dd3d207c06ba4f31d48a5bc4bbcfdc6435fd4d076796f8ef870b`

Descriptor schema/domain stay v1; DNA and portrait traits stay v1; manifest and renderer are v2. Production resolution stays closed. The existing default descriptor and existing v1 fighters/roots remain unchanged.

## Byte evidence

The corpus has 764 full canonical manifest/SVG cases. Its predeclared first-coverage scan covers all 512 first-name and all 512 surname indices by positive integer seed 3739. Fixed zero, incrementing, all-ff and integer seeds are retained. All three actual apostrophe surnames occur and require `&apos;` in both SVG label positions. Every case verifies unchanged DNA/physical/style/portrait values and an identical graphics body relative to the archived v1 renderer.

It also contains 24 literal name-stream preimage/digest/counter vectors, label-order and max-counter exhaustion checks, bounded scripted rejection/exhaustion cases, 18 invalid manifest inputs, 4 invalid renderer/manifest selections and 7 mixed descriptor tuples. OpenSSL independently confirmed all 24 stream hashes and the descriptor hash. No production Go output was read or used as an expected value.

## Collision scenarios and observed sample

Theory assumes independent uniform draws. Observations use exactly the separately predeclared SHA256-derived public seed stream and are not admitted mining work or a forecast. The same input prefixes are used for both versions. This table reports the fraction of fighters that have at least one namesake, not merely the chance that any collision exists somewhere.

| Population | v1 model namesakes | v1 observed | v2 model namesakes | v2 observed | v2 distinct names |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 10 | 0.8755% | 0.0000% | 0.0034% | 0.0000% | 10 |
| 32 | 2.9834% | 0.0000% | 0.0118% | 0.0000% | 32 |
| 100 | 9.2196% | 2.0000% | 0.0378% | 0.0000% | 100 |
| 1,000 | 62.3208% | 61.3000% | 0.3804% | 0.0000% | 1,000 |
| 10,000 | 99.9943% | 100.0000% | 3.7425% | 3.9900% | 9,799 |
| 100,000 | 100.0000% | 100.0000% | 31.7141% | 31.9690% | 82,987 |

The v2 sample happens to have no repeated names in its first 1,000 inputs. That is a sample outcome, not a guarantee or a change to D51. At 100,000 inputs, 31,969 sample fighters have a namesake; a finite expanded pool still repeats. The full JSON also reports probability of any collision, expected/observed equal-name pairs, extra entries and component frequencies. Probability rounded to one is separated from mathematical certainty. No collision threshold, population forecast, demographic weighting, trademark clearance or product-release approval is inferred.

## Retained evidence and next use

`independent/attempt1/` contains generation/check timings and the secondary hash check. `independent/reference_v2.py --check` fully regenerates expected bytes; `--sample --check` fully regenerates the separate collision report. All semantic inputs and scripts are pinned by `independent/input-lock.json`.

Archive the approved contract files, inherited files and compact table with the same relative paths. Archive the independent oracle and its inputs/corpus as the expected-vector provenance. Explanatory repository docs should link to these frozen bytes rather than rewrite their contents or hashes. Runtime implementation, explicit fresh-source selection and integration acceptance remain separate work. The fight engine contract is unchanged; altered manifests/commitments can alter InputID-derived RNG outcomes, so identical trait fields do not promise identical substituted fights.
