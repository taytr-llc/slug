# Fighter transfer policy — D83

Approved by Mike on November 19, 2026 Pacific. This records the product rule;
it is not a claim that transfers have been implemented or released.

> Chain order and checkpoint finality. First competing send in chain order wins,
> later ones are void, transfers display as pending until checkpoint plus 60
> blocks, and noon locks use the owner of record at lock time.

The first valid competing transfer of the same ownership state in canonical
block and transaction order wins. Later competing transfers are void. Service
acceptance, sender timestamps and mempool arrival cannot decide ownership.
The chosen transfer remains visibly pending until its signed checkpoint and
sixty descendant blocks, excluding its inclusion block, establish finality.
That is roughly two hours at the target interval, plus any checkpoint delay.

Before finality, the current owner remains effective. D76's noon-Pacific event
snapshot uses the owner of record at that lock. A transfer still pending at noon
does not move that week's locked fighter to the recipient's gym. Later finality
does not rewrite the event, the original result corner or historical attribution.

Pending state follows canonical chain evidence before finality. Once finalized,
checkpoint-crossing conflict is reported and resolved socially under D62; the
official finalized history is retained. Signed checkpoints remain declarations
of the official record and never reject a node-consensus branch.

Mike chose this so anyone can resolve competing sends from public evidence and
continue the history after a fork or operator failure. Immediate service
acceptance would instead make that service the ordering authority. The service
may verify and relay transactions, but its private arrival queue is not a source
of permanent ownership.

## Existing rules retained

- D61 allows direct wallet-to-wallet gifts and off-market deals without the
  official marketplace commission. Ordinary network transaction fees are
  separate under D33.
- D27 permits unlimited ownership and ten official roster slots. Acquiring a
  fighter does not invent an extra slot or automatically change a noon snapshot.
- D38 preserves the fighter's record and points and requires a professional win
  under the new gym for title-draw requalification.
- D72 blocks hosted fighter transfers during the 24-hour reset hold. It does
  not revoke a previously exported independent wallet copy.
- D48/D70 require tested export, restored gym identity and movement of fighters
  using a SLUG-aware external wallet. A phrase restores keys; public authority
  and transfer history establish the current owned set.

## Engineering work still required

The next functional milestone combines authoritative ownership, actual reviewed
direct transfer and independent same-gym/fighter restoration. Initial ownership
from accepted admission is a prerequisite, not a claim of completed transfers.
The exact versioned carrier, signed payload, public admission/history evidence,
bounded replay and checkpoint records require a reviewed engineering contract.
Existing candidate assignment/history bytes retain their original meaning.

Acceptance must cover competing sends, unchanged-byte retry, restart,
pre-finality reorg, exact checkpoint/depth boundaries, later reported conflict,
noon ownership snapshots, held hosted refusal, and a transfer signed from an
independently restored gym key without hosted login. Preserve birth identity,
appearance, prior result attribution and finalized awards. Wallet recovery must
not fabricate missing payment journals or prior roster/event choices.

Login/provider/MFA implementation remains deferred until these foundations are
complete. The joint D74 design and navigation review remains last.
