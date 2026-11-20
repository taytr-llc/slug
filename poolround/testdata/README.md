# Independent round arithmetic

`reference.py` uses Python's standard `fractions.Fraction` to derive exact gross
atom entitlements and immutable windows from modeled accepted-share events.
The fixed target gives every share equal difficulty weight. The corpus covers
two gyms, fractional rewards, an open tail and a late winner appending a round.

These inputs are deliberately modeled accounting projections, not mined work,
issued jobs or chain authority. Concrete-source tests separately use existing
independently frozen normal-PoW inputs through the real `poolshare` journal.
No runtime Go output is used as the expected arithmetic.
