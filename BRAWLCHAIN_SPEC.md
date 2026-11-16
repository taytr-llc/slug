# Brawlchain: Consolidated Design Spec

Version 1.0, November 14, 2026. Consolidates Decisions 1 to 63, settled with Mike Taylor on November 14, 2026, plus Decisions 64, 65, and 66 added November 15, 2026. Decision numbers are cited as (D#) so any rule can be traced back to the decision log in `~/memory/groups/brawlchain.md`.

## 1. The product

**Pitch:** You run a gym, your fighters train all week, on Saturday they fight at The Mine Event, and the good ones are worth real SLUG.

- Fighters are mined. The proof-of-work result hash is the fighter's DNA: it determines his attributes, physical traits, name, and portrait.
- The miner is reframed as the gym: the browser tab or app where fighters train all week and prospects walk in. Mining never appears in the user-facing story.
- Professional fights happen only at the weekly Saturday fight night, The Mine Event. Exhibitions can happen anytime (D13).
- A professional record is earned history. Time is the anti-farm mechanism at the record layer: records take real Saturdays.
- **No payouts for winning.** Prizes are belts, rankings, and bragging rights. No prize pool, no stakes. Fights decide rankings and the market decides prices; the two never touch.
- Fighters are tradable for SLUG. This is commerce, not gambling, because no outcome pays out. Fighters and SLUG are **never marketed as an investment**, and no appreciation is ever promised.

**Names:** Brand and coin: SLUG, adopted Nov 15, 2026 as the single name for both (originally KOIN, knockout coin, dropped after the Nov 14, 2026 trademark check found the Koinos conflict; full name search in TRADEMARK_DOMAIN_CHECK.md). League working title: Brawlchain (project codename; it is a likely conflict as a public name, so the public brand is built on SLUG). Saturday card: The Mine Event. Logo: open under the SLUG brand (it was a chain under Brawlchain). Rejected: Fight Club (Disney trademark risk), Fighting Ultimate Championship / FUC (UFC confusion, profanity ceiling), and any name leading with "Crypto" (scam optics).

**Status:** Still a design, not a committed build. Build approach is settled: build the actual chain and production system once, then run the small-group test on that real chain. No disposable test product. Internal fight simulations and load testing before genesis are tuning, not a throwaway version.

## 2. Launch and genesis

- **Public mining and public league from genesis** (D1). No invite gate on Season 0.
- **Permanent from genesis** (D2). SLUG, fighters, records, rankings, and titles count from day one. Bug fixes and rule changes apply going forward only; completed history is never rewritten.
- Because the tester phase runs on the real chain, the tester-phase inaugural champions are the real inaugural champions in permanent history (D20).

## 3. Fighters: generation

- **Five core attributes: Power, Speed, Chin, Stamina, Skill** (D3). Skill is the combined home for technique, ringcraft, and heart.
- **Hybrid generation** (D4): rarity first sets a talent budget; the hash then allocates that budget across the five attributes.
- **Each attribute is rated 1 to 100** (D5).
- **Balanced talent pyramid** for regular mined prospects (D6):
  - Club: 55%, total attributes 250 to 299
  - Professional: 27%, 300 to 349
  - Contender: 12%, 350 to 399
  - Elite: 5%, 400 to 449
  - Generational: 1%, 450 to 489
- **Fighting style is derived** from attributes and physical traits (D7). It is a readable label and tendencies computed from the same public numbers, not a hidden sixth attribute. Examples: high Power and Chin with lower Speed is a slugger; high Speed and Skill is an outfighter; high Speed and Stamina is a pressure fighter.
- **Physical traits are generated from the hash and matter in the simulation** (D8): height, reach, stance, and natural weight.
- **Five weight classes** (D9):
  - Light: up to 145 lb
  - Welter: 146 to 170 lb
  - Middle: 171 to 195 lb
  - Light Heavy: 196 to 225 lb
  - Heavy: 226 lb and above

## 4. Fighters: identity

- Identity is mined, not designed. Name comes from hash-driven tables; portrait comes from layered deterministic traits. Same seed, same fighter forever, independently regenerable by anyone.
- **No user customization of fighters.** Human expression lives in gym and stable branding. **Nicknames are earned** from record events, never typed.
- **Duplicate names are allowed** (D51). A fighter's identity is his unique fighter ID, portrait, and record, not the name string alone.
- **Generator version locking** (D55): each fighter keeps the generator version he was mined with, recorded with the fighter. His name, portrait, and traits always regenerate from his seed using that version. Generator updates affect only newly mined fighters.

## 5. Onboarding and prospect discovery

- A new person needs a fighter almost instantly or the product is boring. The first session is a **tryout**: the device performs deliberately easy work, tuned to produce a genuine mined fighter in under a minute, with generated name, portrait, and attributes. The starter's potential is capped in the rookie range, one per gym, nontransferable, and eligible for the rookie division on Saturday.
- **Prospect discovery target: one new prospect every 2 to 3 days for a typical one-device gym** (D30). At that pace: about 10 to 12 prospects a month, an Elite or Generational discovery (6% combined) roughly every 6 to 8 weeks, a Generational (1%) roughly every 8 to 10 months. Bigger miners get proportionally more attempts; roster and entry caps bound the league impact.
- **No prospect signing step and no weekly signing limit** (D44). A discovered prospect is owned immediately.
- Gym time does not raise an existing fighter's base attributes. Gym time discovers better raw talent. Saturdays turn talent into proven value through records.

## 6. Fight rules

- **3 rounds for standard fights, 5 rounds for title fights** (D10).
- **One published scorecard for the whole fight** decides a non-finish (D11), based on aggregate effective damage, knockdowns, accuracy and meaningful volume, and control. It is whole-fight scoring, not round-by-round judging. A tie on the scorecard is a draw.
- **Form modifier: -10% to +10% per fighter per fight** (D12), a single modifier seeded from a future block hash and the bout ID after entries lock. It changes effective performance for that fight only, never permanent attributes. Featured bouts use the per-round seed model instead (D46).

## 7. Matchmaking and the Saturday card

- **The Mine Event is league-wide** (D28). Every entered rostered fighter who can be matched in his weight class and rating pool gets a professional fight that Saturday. If no eligible opponent exists, he has no bout that week; that is not a loss, and he gets matching priority the next week.
- Title fights and the top ranked bouts form a **featured card** on top of the full league results. The featured card is the shared show everyone watches.
- **A gym may enter up to 5 fighters per Saturday, at most one per weight class** (D29).
- **Professional entries are free** (D34). SLUG is not charged to enter a fighter.
- **Matchmaking combines league control with randomness** (D13): the league builds an eligible opponent pool by weight class, rating, and record, then the pairing is drawn randomly from that pool, including step-up opponents so a fighter can move up.
- **Matchmaking pool is a ratio band** (D52): opponents rated between half and double the fighter's matchmaking rating, widening until at least 5 opponents are eligible.
- **The league rating is a weighted sum on current condition** (D65, added Nov 15, 2026): rating = 2.5 x Speed + 1.0 x Power + 0.8 x Skill + 0.4 x Stamina + 0.2 x Chin, computed on current-condition attributes (aging applied), with Skill including permanent pro XP and excluding form and the weekly training bonus. It is published on the fighter page and used for the D52 band. The weights were fitted by logistic regression on 6,000 simulated bouts (rating_study.py): the weighted rating predicted winners at 82.8% on held-out bouts against 73.8% for the raw attribute total, and its correlation with actual career win rate was 0.64 against 0.17 for totals. Rankings remain points-based (D41); there is no Elo. If the fight engine is ever rebalanced, the weights are refit before the new engine ships.
- **Entries lock Saturday at noon PT; results are published Saturday evening** (D45). Saturday morning still counts as a training day, so the final weekly exhibition bonus point can be earned before the noon lock.
- **Title bouts get a dedicated live page** where people watch the featured bouts unfold on Saturday evening (D45).
- **Featured bouts are seeded round by round from new blocks as the live show reaches each round** (D46). Each round is written as it ends, and the official result is written as the bout ends, so nobody, including the league, can compute a featured result in advance. Tradeoff: block times vary, so live pacing follows the chain. Pairings and the title challenger are drawn from the noon lock block, and the undercard uses the noon seed.

## 8. Exhibitions and experience

- **Exhibitions:** owners can challenge each other anytime, including friends, in non-professional matches (D13). Exhibitions do not count toward the professional record, rankings, or titles, but each result goes on the fighter's separate personal exhibition record.
- **Weekly training bonus** (D14, D15): exhibitions build a training bonus of up to **+5 effective Skill**, earned at most **+1 per fighter per day**, applying to that Saturday's professional fight. **It resets every week** after The Mine Event. It does not alter mined base attributes.
- **Professional experience is permanent, capped at +15 effective Skill** (D16), separate from the weekly training bonus. Maximum combined Skill bonus on fight night is +20.
- **Experience earning rate** (D42): +1 per completed professional fight, win or lose, and +2 for a title fight, up to the +15 cap.

## 9. Rankings and titles

- **Rankings in each weight class use a published points system** based on professional results and opponent quality (D17). Anyone can recompute the standings from public fight history.
- **Ranking point values** (D41): win by finish 14, win by decision 10, draw 4, loss 0, plus a fixed quality bonus on a win of +6 for beating a top 5 opponent or +3 for beating an opponent ranked 6 to 10, using the rankings at fight time.
- **Rankings use a rolling 26-week window** (D24). Only results from the last 26 weeks count toward current rankings. Lifetime record, titles, and history remain permanent; they just stop propping up current standing.
- **Title challengers are drawn automatically from the top 5 eligible contenders** (D18, D19). Neither the league nor the champion hand picks the challenger. A losing challenger must win a non-title professional fight before re-entering the title draw, and is also out of the draw for 4 weeks after the loss (D64, added Nov 15, 2026 after the genesis season simulation showed a losing challenger recycling back into a title fight within two weeks).
- **A fighter needs at least 3 professional fights before entering the title draw** (D40).
- **A champion must defend at least once every 2 weeks or the belt is vacated** (D39). A champion who retires vacates immediately. A vacant belt is decided by the number one contender against an opponent drawn from ranks 2 to 5.
- **Inaugural champions** (D20, tester phase): after four ranking weeks, the number one fighter in each class fights an opponent drawn from ranks 2 to 5 for the first belt.
- **Transferred fighters** (D38): lifetime record and ranking points travel with the fighter, but he must win one professional fight under the new gym before entering the title draw. This blocks buying a title shot outright.

## 10. Aging and retirement

- The career clock starts when the fighter is mined, not at debut, so benching cannot preserve prime.
- **Prime length varies from 26 to 52 weeks by hash** (D21).
- **Decline lasts 13 to 39 weeks by hash, then forced retirement from professional fights** (D22). Total career length is 39 to 91 weeks.
- **Total attribute loss by the end of decline** (D43), applied gradually week by week, relative to mined values: Speed -40%, Stamina -30%, Chin -30%, Power -20%, Skill -10%.
- Original mined attributes never change. Aging applies a transparent current-condition adjustment. Experience can offset physical decline up to its cap.
- **Fight wear does not accelerate aging** (rejected as too complex). Damage matters inside the current fight only.
- **Nobody loses their fighter.** Ownership is permanent. Retired fighters can still fight exhibitions for fun and can still be sold or traded as historical fighters. They keep portrait, record, titles, and Hall of Fame history.
- There is no attribute floor at the end of decline (D23, closed as moot): the professional career simply ends when the decline period ends.

## 11. Gyms

- **Every account automatically gets a personal gym** (D25), created with the account and tied to its wallet. The owner names and brands it; branding is the personal expression layer. Gyms are found through a public directory searchable by gym name, links from rankings, fight cards, and fighter pages, and direct invite links. A new person has a gym and a first fighter inside the first minute, with no setup step.
- **People can also join gyms** (D25). Joining is an added affiliation, not a replacement for the personal gym.
- **Joining grants social affiliation plus team standings, with no control over members' fighters** (D26). A joined gym gets a shared banner and page, a member directory, visiting and exhibition challenges. Each owner alone decides which of his fighters train, enter, transfer, or sell, and receives his own marketplace money.
- **10 official roster spots per personal gym** (D27). Ownership remains unlimited. Only rostered fighters are eligible for professional fights and the weekly training bonus. Fighters off the roster can still be traded and fight exhibitions. Joining other gyms does not pool or expand anyone's roster.
- **A person can belong to one team gym at a time, and can leave and switch anytime** (D47). The personal gym is separate and always theirs.
- **Team standings: one board, scored by the average of all members' ranking points** over the rolling 26-week window (D53). Every member counts; size alone gives no advantage; anyone can verify the math by hand.
- **A gym needs at least 5 members to appear on the board** (D54). Gyms under 5 members still exist, recruit, and play; they are just not ranked.
- **Monthly team exhibition event with its own trophy** (D59), kept off professional records.

## 12. SLUG and the chain

- **Hard cap of 21 million SLUG, halving schedule, no premine** (D31).
- **Blocks target 2 minutes; starting block reward is 10 SLUG; the reward halves every 1,050,000 blocks** (D32). A 1,050,000 block interval is about 4 years, and the schedule totals exactly 21,000,000 SLUG.
- **Network transaction fees go to the miner who mines the block** (D33). The network fee is separate from the marketplace commission.
- **Proof of work is Argon2id at 256 MB per attempt** (D50). Memory-hard and CPU friendly, so browser tabs, phones, and laptops can all run it, and memory cost narrows the GPU advantage. Time cost is 1 pass and parallelism is 1 lane per attempt (settled Nov 15, 2026 after benchmarking 64 MB against 256 MB: 66 ms against 340 ms per hash, 4.8 against 24.8 hours of PoW verification per year of chain, with the prospect cadence unchanged because difficulty retargets; see ARGON2ID_PARAMS.md). Miners parallelize across attempts, not within one.
- **Chain architecture: fork a proven open source chain codebase and add the fighter layer** (D49). Not a fresh chain from scratch, not a proof-of-stake modular framework.
- **The codebase is btcd** (D66, added Nov 15, 2026): fork btcsuite/btcd (Go), swap the proof of work to Argon2id at 256 MB per attempt, and set the D32 parameters (21M cap, 10 SLUG start reward, halving every 1,050,000 blocks, 2-minute blocks, fees to the miner). The Bitcoin RPC and stratum surface stays, so pool and explorer tooling adapts; the wallet is btcwallet from the same suite; one Go stack covers the node, the miner, and the Mac and Windows gym apps. Full evaluation, including the ruled-out candidates (Litecoin Core, Kryptokrona, Argonmine source, Tari) and the stated risks (Argon2id verification cost at sync, difficulty tuning for a slow hash, a pre-genesis security review of the swap), is in CHAIN_EVALUATION.md.
- **Difficulty retargets every 2,016 blocks** (D67, decided Nov 15, 2026): TargetTimespan is 2,016 blocks at 2 minutes, about 2.8 days, not Bitcoin's 14-day timespan (which at 2-minute blocks would be a 10,080-block window). The window is counted in blocks, so slow blocks stretch the wall-clock wait: if half to three quarters of the network's hash power leaves, a 10,080-block window leaves blocks crawling for roughly 4 to 8 weeks before difficulty eases, against about 1 to 2 weeks at 2,016 blocks, and SLUG's weekly cadence (Saturday form seeds from a future block hash, prospect discovery, daily pool payouts) rides on steady 2-minute blocks. Precedent: Litecoin has run a 2,016-block window at 2.5-minute blocks (about 3.5 days) since 2011. The per-retarget adjustment stays clamped at 4x, as in Bitcoin. Per-block difficulty algorithms (ASERT, LWMA) were passed over to stay close to btcd's proven difficulty code. Change set 2 shipped with the 10,080-block window; D67 landed in change set 3 on Nov 15, 2026.
- **Wallet custody: accounts start with a hosted wallet behind a normal login, with full export to self custody available anytime** (D48), including revealing the seed phrase and moving fighters and SLUG to an external wallet. Export must be real and tested from day one.

## 13. The official pool

- **The official gym pools mining earnings by default** (D35), splitting block rewards proportionally to shares with regular on-chain payouts. **Solo mining remains available** to anyone outside the pool. Prospects come from a gym's own shares and belong to that gym under either model.
- **The official pool charges 1%, collected by the company because it operates the pool** (D36).
- **Payouts are daily, with a 1 SLUG minimum balance** (D58). Balances under 1 SLUG roll forward.
- The pool is company-run infrastructure. Safeguards: it is optional, payouts are public on chain, and neither the chain nor solo mining depends on it.

## 14. Finality and governance

- **Results are live at once as provisional, and final once buried under 60 blocks** (D62), about 2 hours at the 2 minute target. The league publishes a signed public checkpoint. If a reorganization ever crossed a checkpoint, the checkpointed history stands as the official record and the community decides the canonical branch.
- **Upgrades are opt in** (D63). The company proposes and publishes upgrades; node operators and gyms choose whether to run them. League rule changes are announced at least 4 weeks before taking effect, and are never retroactive.
- **Fork doctrine:** because the chain is public, anyone can snapshot it, import every fighter and every historical result into a fork under a new name, and continue those copied records on that fork's own path. History is public and portable; canonical continuation is social. What a fork cannot automatically take is recognition: owners entering copied fighters, miners securing the branch, buyers trading there, and viewers treating its Saturday card as real. Working principle: open code, protected brand, sanctioned championship. Forks are also insurance if the operator ever becomes another Argonmine maintainer.

## 15. Marketplace and revenue

- **Sale format at launch is fixed price listings only; auctions come later** (D37), once there is a public price baseline and enough traffic. Fixed price sales create a clean public price history.
- **Listings always show age in weeks, career phase, current effective attributes, and prime remaining** (D56). Sellers cannot hide them. All of it is public chain data anyway; full transparency is the marketplace's trust engine.
- **Fighters are bought with SLUG only at launch** (D60). Dollar checkout comes later through a licensed partner, with fighter prices and settlement staying in SLUG. The company never sells SLUG directly.
- **Direct wallet to wallet transfers are allowed, with no commission** (D61). Gifts and off-market deals are legitimate. The record, points, and title requalification rules apply however a fighter changes hands.
- **Company revenue:**
  - **5% commission on sales through the official marketplace.** A fighter sells for 100 SLUG, the seller receives 95, the company receives 5. Ordinary SLUG transfers are not taxed.
  - **Paid gym features:** presentation, convenience, roster management, and scouting tools. **No paid attribute boosts, ever.** Priced in dollars only at launch; SLUG payment is added once a real market rate exists (D57). At genesis SLUG has no dollar price, so there is no rate to accept it at.
  - **Advertisers in the gym:** wall posters, ring aprons, corner branding, sponsored fight cards.
  - **The official pool's 1% fee** (D36).
  - **Company mining** (below).
- **Company mining:** the company may run its own mining operation under exactly the same rules as everyone else, earn SLUG, and sell it once a market exists. Hard lines: **no premine and no privileged mining.** No genesis allocation, no private faster miner, no special emission rate. Company mining is bootstrap infrastructure and liquidity, not the main business. Proposed safeguards, not all separately ratified: a public company wallet, the public client, disclosed sales, and a declining share of total network power as independent gyms join.

## 16. Farm resistance

Nothing completely stops farms in a permissionless work system. The design goal is narrower: let farms farm SLUG without letting farmed SLUG convert directly into league control.

- Fixed issuance schedule and cap: more hash power raises difficulty rather than creating extra SLUG.
- SLUG cannot improve attributes, buy rankings, or buy title shots.
- Roster cap of 10 (D27). Five Saturday entries, at most one per weight class (D29). One fight per fighter per Saturday.
- Matchmaking ratio band (D52), so farmed prospects cannot be fed hand-picked weak opponents.
- Aging and forced retirement (D21, D22, D43), so no generation rules forever.
- Transfer title requalification (D38), so a bought contender cannot take an immediate title shot.
- No signing step (D44): the roster and entry caps are the league gate.
- Miners cannot take other gyms' fighters; they can only sell fighters they mined themselves.
- Majority-hash attacks are a separate chain-level risk. Settled mitigation: finality checkpoints (D62) plus community recognition of the canonical branch. Open mitigations: difficulty adjustment specifics and seed node distribution (section 17).

## 17. Open items: build-time tuning and pre-launch work

These are not unsettled decisions. They are engineering tuning or work items that do not change the rules above.

**Build-time tuning:**
- Fight engine internals: damage model, knockdown and finish mechanics, draw handling beyond the settled scorecard, exact form modifier application for undercard bouts. (Matchmaking band width is settled in D52 and the rating formula in D65.)
- Fighter DNA details: physical trait ranges and class population shaping, name and portrait generation tables.
- Chain details: public seed nodes, public explorer and independent nodes, fighter ownership and marketplace settlement formats, official entry and result publication formats. (Difficulty adjustment is settled in D67; the codebase in D66 and the Argon2id parameters with it.)
- Team event format details beyond D59.
- Marketplace details: wash trading and self-dealing display rules, listed fighter entry and aging edge cases, auction design for later.
- Premium feature catalog beyond the categories in section 15.

**Pre-launch work:**
- Trademark and domain checks: knock-out checks done Nov 14 to 15, 2026 (see TRADEMARK_DOMAIN_CHECK.md); a formal trademark attorney opinion and filings for SLUG and The Mine Event remain pre-launch work. Open SLUG domains found: slugleague.com, slug.game, slug.money, slug.gold, slug.co.
- Legal review: operating company, terms and privacy, eligible ages and countries, securities, gambling, tax, and marketplace review, gym naming and advertising moderation, no investment language anywhere.
- Operations: public seeds and nodes, backups, monitoring, release process, security audit, incident and bug response, support.
- Client: the gym ships as a browser app plus native Mac and Windows apps (menu bar / tray, gentle default mining setting), decided Nov 15, 2026; browsers throttle background tabs and laptop mining is audible, so the native apps are the uptime answer. Signing and notarization run through Mike's developer accounts.

**Explicitly deferred:** cash checkout (D60 covers timing), advertiser sales program, merchandise, advanced analytics, extra tournament formats, most cosmetic gym features, provided the core data model does not block them.

**Build plan:** workstreams, dependencies, and sequence are consolidated in BUILD_PLAN.md (added Nov 15, 2026). The official pool (section 13) is workstream 3 there.
