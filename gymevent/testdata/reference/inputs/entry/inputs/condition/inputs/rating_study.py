#!/usr/bin/env python3
"""Rating formula study for the SLUG league (Brawlchain).

The spec leaves the rating formula open (D13, D53 use a rating for
matchmaking; the prototype has been running on raw attribute totals,
then Elo-adjusted totals). This study settles it with sim data:

  Phase 1: fit per-attribute weights by logistic regression on 4,000
           prime bouts (what is a point of each attribute worth?).
  Phase 2: on 2,000 held-out bouts, compare predictors of the winner:
           raw total, weighted total, Elo from total, Elo from weighted.
  Phase 3: on 1,500 bouts where one fighter is mid-decline, compare
           rating on mined attributes vs current-condition attributes.
  Phase 4: revisit the 40-career prime records and check which formula
           separates the killers from the cans (the Club killer and
           0-win Professional problem).

Run: python3 rating_study.py
"""

import math
import fight_sim as fs

ATTRS = fs.ATTRS

def bout_outcome(fa, fb, seed):
    res = fs.simulate_bout(fa, fb, seed, verbose=False)
    if res.winner is None:
        return 0.5
    return 1.0 if res.winner is fa else 0.0

def sigmoid(x):
    return 1.0 / (1.0 + math.exp(-max(-30.0, min(30.0, x))))

def fit_weights(samples, epochs=400, lr=0.5):
    """samples: list of (diffs list of 5, y in 0/0.5/1). Returns weights
    on the raw attribute scale, normalized so the mean weight is 1."""
    w = [0.0] * 5
    n = len(samples)
    for _ in range(epochs):
        grad = [0.0] * 5
        for diffs, y in samples:
            p = sigmoid(sum(wi * d for wi, d in zip(w, diffs)))
            for i in range(5):
                grad[i] += (p - y) * diffs[i]
        for i in range(5):
            w[i] -= lr * grad[i] / n
    mean_w = sum(w) / 5.0
    return [wi / mean_w for wi in w]

def weighted(attrs, weights):
    return sum(weights[i] * attrs[a] for i, a in enumerate(ATTRS))

def sign_score(pairs):
    """pairs: (rating_a, rating_b, y). Credit 1 if the higher rated
    fighter won, 0.5 for a draw or a rating tie, else 0."""
    tot = 0.0
    for ra, rb, y in pairs:
        if ra == rb:
            tot += 0.5
        elif (ra > rb) == (y == 1.0):
            tot += 1.0
        elif y == 0.5:
            tot += 0.5
    return tot / len(pairs)

def main():
    print("Generating 6,000 prime bouts...")
    samples = []
    fighters = []
    for i in range(6000):
        fa = fs.generate_fighter(f"rs-{i}-a")
        fb = fs.generate_fighter(f"rs-{i}-b")
        y = bout_outcome(fa, fb, f"rs-bout-{i}")
        diffs = [(fa.attrs[a] - fb.attrs[a]) / 50.0 for a in ATTRS]
        samples.append((diffs, y, fa, fb))
        fighters.extend([fa, fb])
    train = [(d, y) for d, y, _fa, _fb in samples[:4000]]
    test = samples[4000:]

    weights = fit_weights(train)
    print("\nPhase 1: fitted attribute weights (mean = 1.00, raw total uses 1.00 for all)")
    for a, w in zip(ATTRS, weights):
        print(f"  {a}: {w:.2f}")

    print("\nPhase 2: held-out prediction (2,000 bouts), higher score is better")
    raw_pairs = [(fa.total, fb.total, y) for _d, y, fa, fb in test]
    wtd_pairs = [(weighted(fa.attrs, weights), weighted(fb.attrs, weights), y)
                 for _d, y, fa, fb in test]
    print(f"  Raw total:            {sign_score(raw_pairs):.1%}")
    print(f"  Weighted total:       {sign_score(wtd_pairs):.1%}")
    for label, pairs in (("raw", raw_pairs), ("weighted", wtd_pairs)):
        buckets = {}
        for ra, rb, y in pairs:
            d = abs(ra - rb)
            b = "0-10" if d < 10 else "10-25" if d < 25 else "25-50" if d < 50 else "50+"
            w_, n_ = buckets.get(b, (0.0, 0))
            fav_won = 1.0 if (ra > rb) == (y == 1.0) and y != 0.5 else 0.5 if y == 0.5 else 0.0
            buckets[b] = (w_ + fav_won, n_ + 1)
        print(f"  Favorite win rate by {label} rating gap: "
              + ", ".join(f"{b}: {w_ / n_:.0%} (n={n_})" for b, (w_, n_) in sorted(buckets.items())))

    print("\nPhase 3: declined fighter vs prime fighter (1,500 bouts)")
    mined_pairs, cond_pairs = [], []
    for i in range(1500):
        fa = fs.generate_fighter(f"dec-{i}-a")
        fb = fs.generate_fighter(f"dec-{i}-b")
        import random as _r
        wk = _r.Random(i).randint(fa.prime_weeks, fa.career_weeks - 1)
        eff = fs.effective_attrs(fa, wk)
        res = fs.simulate_bout(fa, fb, f"dec-bout-{i}", base_a=eff, verbose=False)
        y = 0.5 if res.winner is None else 1.0 if res.winner is fa else 0.0
        mined_pairs.append((fa.total, fb.total, y))
        cond_pairs.append((weighted(eff, weights), weighted(fb.attrs, weights), y))
    print(f"  Rating on mined attributes (raw total):     {sign_score(mined_pairs):.1%}")
    print(f"  Rating on current condition (weighted):     {sign_score(cond_pairs):.1%}")

    print("\nPhase 4: the 40 careers, prime records vs formula")
    print("  (Elo variants are tested here, online: predict each fight with")
    print("   the fighter's current Elo, then update it. Opponents enter at")
    print("   their formula seed, since each appears once.)")
    rows = []
    elo_scores = {"total": [0.0, 0], "weighted": [0.0, 0]}
    for i in range(40):
        f = fs.generate_fighter(f"agg-{i}")
        xp = 0
        w = l = 0
        elo_t = float(f.total)
        elo_w = weighted(f.attrs, weights)
        for week in range(f.prime_weeks):
            opp = fs.find_opponent(f, week)
            if opp is None:
                continue
            res = fs.simulate_bout(f, opp, f"agg-{i}-w{week}",
                                   skill_bonus_a=min(15, xp),
                                   base_a=fs.effective_attrs(f, week), verbose=False)
            xp += 1
            y = 0.5 if res.winner is None else 1.0 if res.winner is f else 0.0
            for key, mine, his in (("total", elo_t, float(opp.total)),
                                   ("weighted", elo_w, weighted(opp.attrs, weights))):
                s = elo_scores[key]
                if mine == his:
                    s[0] += 0.5
                elif (mine > his) == (y == 1.0):
                    s[0] += 1.0
                elif y == 0.5:
                    s[0] += 0.5
                s[1] += 1
            ea = 1.0 / (1.0 + 10.0 ** (-(elo_t - opp.total) / 150.0))
            elo_t += 16.0 * (y - ea)
            ea = 1.0 / (1.0 + 10.0 ** (-(elo_w - weighted(opp.attrs, weights)) / 150.0))
            elo_w += 16.0 * (y - ea)
            if res.winner is f:
                w += 1
            elif res.winner is opp:
                l += 1
        if w + l:
            rows.append((f, w / (w + l), w, l))
    for key, (s, n) in elo_scores.items():
        print(f"  Online Elo from {key}: {s / n:.1%} over {n} career fights")
    import statistics
    tot_corr = statistics.correlation([r[0].total for r in rows], [r[1] for r in rows])
    wtd_corr = statistics.correlation([weighted(r[0].attrs, weights) for r in rows],
                                      [r[1] for r in rows])
    print(f"  Correlation with actual prime win rate: raw total {tot_corr:.2f}, weighted {wtd_corr:.2f}")
    rows.sort(key=lambda r: r[1])
    print("  Worst prime records and their two ratings:")
    for f, rate, w, l in rows[:3]:
        print(f"    {f.name} ({f.tier}) {w}-{l}: total {f.total}, weighted {weighted(f.attrs, weights):.0f}")
    print("  Best prime records and their two ratings:")
    for f, rate, w, l in rows[-3:]:
        print(f"    {f.name} ({f.tier}) {w}-{l}: total {f.total}, weighted {weighted(f.attrs, weights):.0f}")

if __name__ == "__main__":
    main()
