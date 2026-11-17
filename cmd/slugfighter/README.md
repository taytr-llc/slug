# slugfighter

`slugfighter` previews the version 1 fighter DNA component from a supplied
32-byte work hash. It runs offline and prints indented JSON followed by a
newline. The output includes the DNA version, canonical lowercase seed,
rarity, talent budget, five attributes, prime weeks and decline weeks.

Build from the repository root in workspace mode:

```sh
go build -o /tmp/slugfighter ./cmd/slugfighter
```

For example, preview these arbitrary test bytes:

```sh
/tmp/slugfighter -dna-version=1 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
```

The version defaults to `1`; other versions are rejected. Supply exactly one
64-character hexadecimal hash, without a `0x` prefix or whitespace. Uppercase
hex is accepted and emitted in lowercase. Put flags before the hash. Use
`slugfighter -h` for help. Errors go to standard error and return a nonzero
exit status; successful previews go to standard output.

This preview accepts any 32 bytes. It does not mine, validate qualifying work,
sign anything, register a fighter, or grant fighter ownership. It uses no
network connection or private keys. The original DNA route does not include
names, portrait selections, physical traits or gym registration.

The optional `preview` subcommand adds the candidate static manifest while
preserving the original DNA command:

```sh
slugfighter preview --work-hash <64-hex-work-hash>
slugfighter preview --work-hash <64-hex-work-hash> --digest
```

The default preview output is exact canonical JSON without a terminal newline;
`--digest` prints its separate digest. `--version` defaults to the explicit
manifest version 1 and rejects unknown versions. This is an offline public
preview, not qualifying work, registration, a fighter ID or an ownership claim.
See `FIGHTER_MANIFEST.md` at the repository root for the candidate contract.

The `portrait` subcommand renders the candidate manifest as a deterministic
200-by-200 SVG document, without a terminal newline:

```sh
slugfighter portrait --work-hash <64-hex-work-hash>
slugfighter portrait --work-hash <64-hex-work-hash> --digest
```

`--version` selects the explicit renderer version (only 1 is supported);
renderer 1 accepts manifest 1. `--digest` prints the separate SVG digest plus
a newline. This command takes public hash text only, constructs all SVG bytes
and accepts no caller markup. The digest is not a fighter ID or proof.
See `FIGHTER_PORTRAITS.md` for frozen bytes and accessibility wording. Browser
and screen-reader acceptance remain separate from exact-byte conformance.

The `condition` subcommand previews aging at explicitly supplied times:

```sh
slugfighter condition --work-hash <64-hex-work-hash> --born-at-ms <unix-ms> --as-of-ms <unix-ms> --pro-xp 0
```

The birth must be positive, observation time must not precede it, and accounted
XP must be 0–15. There is no implicit current clock. `--version` accepts only
projector 1. Indented JSON reports aged and XP-adjusted attributes as integer
tenths and the D65 rating in hundredths. These units are explicit in the field
names. This command authenticates no birth, timestamp, XP, admission, ownership
or roster eligibility. See `FIGHTER_CONDITION.md`; original DNA stays intact.
