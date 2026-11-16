// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package fighter derives reproducible fighter DNA from a proof-of-work result
// using a versioned, pure function. A seed is the 32 bytes obtained by decoding
// its displayed/RPC hexadecimal hash, not chainhash's reversed internal order.
//
// Version 1 covers rarity, talent budget, five base attributes, and prime and
// decline lengths. It is a DNA-component version, not a completed fighter
// generator: names, portraits, physical traits, identity, ownership, prospect
// eligibility, and registration are outside this package. Generating DNA from
// an arbitrary seed does not establish that anyone mined or owns a fighter.
//
// Generation uses SHA256 streams separated by version and field name. This
// conditions the seed before sampling: hashes qualifying against a PoW target
// are not uniformly distributed over the entire raw hash range. Conditioning
// avoids that encoding bias; it does not prevent miners from evaluating their
// own candidate seeds or withholding discoveries.
//
// The generator has no network, clock, randomness source, mutable configuration,
// or floating-point dependency. Existing version mappings must remain fixed;
// changing the generation rules requires a new explicitly selected version.
package fighter
