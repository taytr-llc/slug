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
network connection or private keys. Names, portraits, physical traits and gym
registration are future work with separate versioning; this command is not a
complete fighter generator.
