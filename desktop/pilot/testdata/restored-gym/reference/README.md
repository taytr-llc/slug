# Independent restored-gym setup reference

`setup-v1.json` is SHA256 `3d31e7f195fa20456978739258f5517fbe2a4d97d93b379c9b8ade941daa3147`.
Its 24 positive, 54 semantic-refusal and 48 parser-refusal cases were generated
from the reviewed Python model and public inherited inputs, before the new Go
setup implementation. Python 3.14.3 and 3.9.6 reproduced identical bytes.

Run `PYTHONDONTWRITEBYTECODE=1 python3 reproduce.py` from any directory. The
producer checks the exact root-approved lock, its closed source set and all 11
catalogued public input files before importing any independent model. Original
absolute source paths in the catalog are provenance, not runtime dependencies.
No product Go, private wallet database, selected real recovery file, node or
provider participates in generation. Published fixture mnemonic strings remain
only in the previously public input corpus; the output contains public metadata.

This proves marker grammar, nested relationships, domains and bounded byte
arithmetic. It does not prove filesystem publication, setup/recovery lifecycle,
opaque capabilities, chain truth, escrow availability or signing.

Three failed first executions were retained externally: recipe key order,
a no-op mutation literal, and missing root-approval metadata. Their successors
preserved all prior source/locks; no failed attempt produced an output. The
accepted endpoint correction keeps the private node literal-loopback HTTP and
allows the existing pinned HTTPS public reader. No schema/crypto oracle came
from new implementation output.
