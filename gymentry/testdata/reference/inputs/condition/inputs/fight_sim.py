#!/usr/bin/env python3
"""SLUG fight simulation prototype (Brawlchain design, Decisions 1-63).

Pre-genesis tuning tool. Everything is deterministic: a fighter is fully
derived from a seed string (standing in for a block hash), and a bout is
fully derived from the two fighter seeds plus a bout seed (standing in
for the Saturday lock block hash and, for featured bouts, per-round
block hashes). Same inputs, same fight, on any machine. That is the
verifiability property the league design depends on.

Implements, per the spec:
  - Five attributes, 1-100, hybrid generation: rarity tier sets a
    talent budget, the hash allocates it (D3-D5).
  - Talent pyramid: Club 55% / Pro 27% / Contender 12% / Elite 5% /
    Generational 1% (D5).
  - Physical traits from the hash: height, reach, stance, natural
    weight, five weight classes (D7-D8).
  - Style derived from attributes and traits (D7).
  - 3 rounds standard, 5 for title fights (D9).
  - One published whole-fight scorecard: effective damage, knockdowns,
    accuracy and meaningful volume, control (D10).
  - Form modifier -10% to +10% per fighter per fight, seeded from the
    bout seed after entries lock (D12); featured bouts reseed each
    round (D46).
  - Optional Skill bonuses (training + pro experience, cap +20) can be
    passed in as effective-attribute adjustments (D42, D52).

Stdlib only. Run: python3 fight_sim.py card | bout SEED_A SEED_B | balance N
"""

import argparse
import hashlib
import random
import sys
from dataclasses import dataclass, field

# ---------------------------------------------------------------- seeds

def seed_int(*parts):
    h = hashlib.sha256(":".join(str(p) for p in parts).encode()).digest()
    return int.from_bytes(h[:8], "big")

def rng_for(*parts):
    return random.Random(seed_int(*parts))

# ---------------------------------------------------------------- names

FIRST = ["Marcus", "Dre", "Tommy", "Hector", "Yuri", "Sam", "Eddie", "Frankie",
         "Leo", "Omar", "Vince", "Ray", "Kenny", "Igor", "Paulie", "Andre",
         "Milo", "Hank", "Rudy", "Cesar", "Otto", "Benny", "Al", "Dmitri",
         "Joey", "Sal", "Nico", "Emil", "Walter", "Gus", "Felix", "Roman"]
LAST = ["Kovac", "Reyes", "Barnes", "Ortiz", "Volkov", "Marsh", "Doyle", "Ferro",
        "Lindqvist", "Haddad", "Crane", "Sosa", "Petrov", "Wolfe", "Brandt", "Iversen",
        "Marek", " Boone", "Keller", "Ashford", "Rinaldi", "Stokes", "Varga", "Holt",
        "Mercer", "Lund", "Parisi", "Grange", "Whitlock", "Sorel", "Brand", "Kessler"]

# ---------------------------------------------------------------- fighter

TIERS = [  # (name, cumulative probability, budget min, budget max)
    ("Club",         0.55, 250, 299),
    ("Professional", 0.82, 300, 349),
    ("Contender",    0.94, 350, 399),
    ("Elite",        0.99, 400, 449),
    ("Generational", 1.00, 450, 489),
]

WEIGHT_CLASSES = [  # (name, max natural weight in lb)
    ("Light", 145), ("Welter", 170), ("Middle", 195),
    ("Light Heavy", 225), ("Heavy", 10**9),
]

ATTRS = ["Power", "Speed", "Chin", "Stamina", "Skill"]

@dataclass
class Fighter:
    seed: str
    fid: str
    name: str
    tier: str
    attrs: dict          # mined attributes, never change
    height_in: int
    reach_in: int
    stance: str
    weight_lb: int
    wclass: str
    style: str
    prime_weeks: int     # D21-D22: career clock starts at mining
    decline_weeks: int

    @property
    def career_weeks(self):
        return self.prime_weeks + self.decline_weeks

    @property
    def total(self):
        return sum(self.attrs.values())

def weight_class(weight):
    for name, cap in WEIGHT_CLASSES:
        if weight <= cap:
            return name
    return "Heavy"

def derive_style(a):
    if a["Power"] >= 70 and a["Chin"] >= 65 and a["Speed"] < 60:
        return "Slugger"
    if a["Speed"] >= 70 and a["Skill"] >= 65:
        return "Outfighter"
    if a["Speed"] >= 65 and a["Power"] >= 65:
        return "Boxer-Puncher"
    if a["Stamina"] >= 70 and a["Skill"] >= 55:
        return "Pressure"
    return "Boxer"

def generate_fighter(seed):
    rng = rng_for("fighter", seed)
    fid = "F-" + hashlib.sha256(seed.encode()).hexdigest()[:8]
    name = rng.choice(FIRST).strip() + " " + rng.choice(LAST).strip()

    roll = rng.random()
    tier, budget = None, None
    for tname, cum, lo, hi in TIERS:
        if roll < cum:
            tier, budget = tname, rng.randint(lo, hi)
            break

    # Allocate the talent budget across the five attributes.
    weights = [rng.uniform(0.45, 1.55) for _ in ATTRS]
    wsum = sum(weights)
    vals = [max(10, round(budget * w / wsum)) for w in weights]
    vals = [min(100, v) for v in vals]
    # Fix rounding/clamping drift so the total lands exactly on budget.
    guard = 0
    while sum(vals) != budget and guard < 500:
        guard += 1
        i = rng.randrange(len(vals))
        if sum(vals) < budget and vals[i] < 100:
            vals[i] += 1
        elif sum(vals) > budget and vals[i] > 10:
            vals[i] -= 1
    attrs = dict(zip(ATTRS, vals))

    weight = int(rng.triangular(128, 268, 178))
    height = int(round(64 + (weight - 128) / 140 * 14 + rng.uniform(-2.5, 2.5)))
    height = max(62, min(80, height))
    reach = height + rng.randint(-2, 4)
    stance = "Southpaw" if rng.random() < 0.25 else "Orthodox"
    prime_weeks = rng.randint(26, 52)      # D21
    decline_weeks = rng.randint(13, 39)    # D22

    return Fighter(seed=seed, fid=fid, name=name, tier=tier, attrs=attrs,
                   height_in=height, reach_in=reach, stance=stance,
                   weight_lb=weight, wclass=weight_class(weight),
                   style=derive_style(attrs),
                   prime_weeks=prime_weeks, decline_weeks=decline_weeks)

def fighter_card(f):
    a = f.attrs
    return (f"{f.name} ({f.fid}) | {f.tier} | {f.wclass} | {f.style}, {f.stance}\n"
            f"  {f.height_in} in, reach {f.reach_in} in, {f.weight_lb} lb | "
            f"POW {a['Power']} SPD {a['Speed']} CHN {a['Chin']} STA {a['Stamina']} SKL {a['Skill']} "
            f"| total {f.total}\n"
            f"  Career shape: prime {f.prime_weeks}w, decline {f.decline_weeks}w, "
            f"retires after week {f.career_weeks}")

# ---------------------------------------------------------------- lifecycle

# D23: total attribute loss by the end of decline, applied gradually.
# Mined attributes never change; this is a published current-condition
# adjustment, exactly what marketplace listings must show.
DECLINE_LOSS = {"Speed": 0.40, "Stamina": 0.30, "Chin": 0.30,
                "Power": 0.20, "Skill": 0.10}

def phase_at(f, week):
    """Career phase at a given week since mining (D21-D22)."""
    if week < f.prime_weeks:
        return "Prime"
    if week < f.career_weeks:
        return "Decline"
    return "Retired"

def prime_remaining(f, week):
    return max(0, f.prime_weeks - week)

def effective_attrs(f, week):
    """Current-condition attributes at a week: mined attrs adjusted
    for aging only. Full strength through prime, a linear slide across
    decline, final declined values after retirement (exhibitions)."""
    phase = phase_at(f, week)
    if phase == "Prime":
        return dict(f.attrs)
    if phase == "Retired":
        p = 1.0
    else:
        p = (week - f.prime_weeks) / f.decline_weeks
    return {k: round(f.attrs[k] * (1.0 - DECLINE_LOSS[k] * p), 1) for k in ATTRS}

# ---------------------------------------------------------------- engine

STRIKES = {  # name: (base damage, is power shot)
    "jab":      (1.2, False),
    "cross":    (2.3, True),
    "hook":     (2.8, True),
    "uppercut": (3.3, True),
    "body":     (1.8, False),
}

STYLE_MIX = {  # strike selection weights by style
    "Slugger":       {"jab": .25, "cross": .25, "hook": .25, "uppercut": .15, "body": .10},
    "Outfighter":    {"jab": .50, "cross": .20, "hook": .10, "uppercut": .05, "body": .15},
    "Boxer-Puncher": {"jab": .35, "cross": .25, "hook": .20, "uppercut": .10, "body": .10},
    "Pressure":      {"jab": .30, "cross": .20, "hook": .25, "uppercut": .10, "body": .15},
    "Boxer":         {"jab": .40, "cross": .25, "hook": .15, "uppercut": .08, "body": .12},
}

TICKS_PER_ROUND = 60          # one tick is about 3 seconds of a 3 minute round
DRAW_MARGIN = 2.0             # scorecard totals this close are a draw

@dataclass
class Corner:
    """Live state for one fighter during a bout."""
    f: Fighter
    base: dict                # attributes the bout is fought with (aging applied)
    form: float               # -0.10 .. +0.10, this fight only (D12)
    eff: dict                 # effective attributes after form and bonuses
    ko_threshold: float
    stamina: float = 0.0
    max_stamina: float = 0.0
    damage_taken: float = 0.0
    thrown: int = 0
    landed: int = 0
    sig_landed: int = 0
    damage_dealt: float = 0.0
    knockdowns: int = 0       # scored against the opponent
    control: float = 0.0
    kd_this_round: int = 0    # suffered this round

    def stam_frac(self):
        return self.stamina / self.max_stamina if self.max_stamina else 0.0

    def hurt_frac(self):
        return self.damage_taken / self.ko_threshold if self.ko_threshold else 0.0

def make_corner(f, base, form, skill_bonus, threshold_jitter):
    eff = {}
    for k in ATTRS:
        eff[k] = base[k] * (1.0 + form)
    if skill_bonus:
        eff["Skill"] = min(120.0, eff["Skill"] + skill_bonus)
    max_stam = 60.0 + eff["Stamina"] * 1.4
    return Corner(f=f, base=base, form=form, eff=eff,
                  ko_threshold=(70.0 + base["Chin"] * 0.45) * threshold_jitter,
                  stamina=max_stam, max_stamina=max_stam)

def pick_strike(rng, style):
    mix = STYLE_MIX[style]
    r, acc = rng.random(), 0.0
    for name, w in mix.items():
        acc += w
        if r < acc:
            return name
    return "jab"

@dataclass
class BoutResult:
    a: Fighter
    b: Fighter
    winner: object            # Fighter, or None for a draw
    method: str               # KO, TKO, Decision, Draw
    end_round: int
    rounds_total: int
    score_a: dict
    score_b: dict
    log: list = field(default_factory=list)

def scorecard(c):
    acc = (c.landed / c.thrown) if c.thrown else 0.0
    parts = {
        "damage": round(c.damage_dealt, 1),
        "knockdowns": round(c.knockdowns * 10.0, 1),
        "accuracy": round(acc * 25.0, 1),
        "volume": round(c.sig_landed * 0.6, 1),
        "control": round(c.control * 0.35, 1),
    }
    parts["total"] = round(sum(parts.values()), 1)
    return parts

def simulate_bout(fa, fb, bout_seed, title=False, featured=False,
                  skill_bonus_a=0, skill_bonus_b=0, verbose=True,
                  base_a=None, base_b=None):
    rounds_total = 5 if title else 3
    form_rng = rng_for("form", bout_seed, fa.fid)
    form_a = form_rng.uniform(-0.10, 0.10)
    form_rng = rng_for("form", bout_seed, fb.fid)
    form_b = form_rng.uniform(-0.10, 0.10)
    jit_rng = rng_for("chin", bout_seed, fa.fid, fb.fid)
    ca = make_corner(fa, base_a if base_a is not None else fa.attrs,
                     form_a, skill_bonus_a, jit_rng.uniform(0.92, 1.08))
    cb = make_corner(fb, base_b if base_b is not None else fb.attrs,
                     form_b, skill_bonus_b, jit_rng.uniform(0.92, 1.08))
    corners = [ca, cb]
    log = []
    winner, method, end_round = None, None, rounds_total
    finished = False

    for rnd in range(1, rounds_total + 1):
        if featured:  # D46: featured bouts reseed from each new block
            rng = rng_for("round", bout_seed, rnd, fa.fid, fb.fid)
        elif rnd == 1:
            rng = rng_for("bout", bout_seed, fa.fid, fb.fid)
        for c in corners:
            c.kd_this_round = 0
        round_start = [(c.thrown, c.landed) for c in corners]
        events = []

        for _tick in range(TICKS_PER_ROUND):
            # Initiative: speed and remaining stamina decide who throws.
            wa = ca.eff["Speed"] * (0.45 + 0.55 * ca.stam_frac())
            wb = cb.eff["Speed"] * (0.45 + 0.55 * cb.stam_frac())
            att, dfd = (ca, cb) if rng.random() < wa / (wa + wb) else (cb, ca)

            strike = pick_strike(rng, att.f.style)
            base_dmg, is_power = STRIKES[strike]
            att.thrown += 1
            att.stamina = max(0.0, att.stamina - (1.15 if is_power else 0.8))

            p_hit = 0.30 + (att.eff["Skill"] - dfd.eff["Speed"]) * 0.0022
            if att.f.stance != dfd.f.stance:
                p_hit += 0.02  # open stance matchup slightly favors the thrower
            p_hit *= 0.72 + 0.28 * att.stam_frac()
            p_hit += max(0.0, 0.55 - dfd.stam_frac()) * 0.10  # tired men get hit
            p_hit = min(0.78, max(0.06, p_hit))

            if rng.random() >= p_hit:
                att.control += 0.5  # pressing forward still banks control
                continue

            att.landed += 1
            dmg = base_dmg * (0.65 + att.eff["Power"] / 100.0) * rng.uniform(0.6, 1.45)
            if strike == "jab":
                att.control += 0.5
            if strike == "body":
                dfd.stamina = max(0.0, dfd.stamina - dmg * 0.9)
            is_sig = is_power or dmg >= 1.6
            if is_sig:
                att.sig_landed += 1
            att.damage_dealt += dmg
            dfd.damage_taken += dmg
            if dmg >= 4.2:
                events.append(f"  {att.f.name} lands a huge {strike} ({dmg:.1f})")

            # Knockdown check on significant shots.
            if is_sig:
                v = dfd.hurt_frac()
                kd_p = (0.008 + max(0.0, dmg - 3.0) * 0.018 + v * v * 0.22
                        - dfd.base["Chin"] * 0.00012)
                if rng.random() < min(0.45, max(0.0, kd_p)):
                    dfd.kd_this_round += 1
                    att.knockdowns += 1
                    att.control += 3.0
                    dfd.damage_taken += 6.0
                    att.damage_dealt += 6.0
                    art = "an" if strike == "uppercut" else "a"
                    events.append(f"  DOWN: {dfd.f.name} is dropped by {art} {strike}")
                    if dfd.kd_this_round >= 3:
                        winner, method, finished = att.f, "TKO (three knockdowns)", True
                        break
                # Referee stoppage once a fighter is badly hurt.
                if dfd.hurt_frac() >= 1.0:
                    winner, method, finished = att.f, "KO", True
                    break
                if dfd.hurt_frac() >= 0.85 and rng.random() < 0.10 + (dfd.hurt_frac() - 0.85) * 1.2:
                    winner, method, finished = att.f, "TKO (referee stoppage)", True
                    break
        end_round = rnd

        if verbose:
            line = [f"Round {rnd}:"]
            for i, c in enumerate(corners):
                t0, l0 = round_start[i]
                line.append(f"{c.f.name} {c.landed - l0}/{c.thrown - t0}")
            log.append("  " + " | ".join(line))
            log.extend(events)

        if finished:
            break

        # Between rounds: corners recover some stamina, damage mostly stays.
        for c in corners:
            c.stamina = min(c.max_stamina,
                            c.stamina + c.max_stamina * (0.10 + c.eff["Stamina"] / 100.0 * 0.10))
            c.damage_taken *= 0.97
        # Corner stoppage between rounds.
        for att, dfd in ((ca, cb), (cb, ca)):
            if dfd.hurt_frac() >= 0.97 and rng.random() < 0.35:
                winner, method, finished = att.f, "TKO (corner stoppage)", True
        if finished:
            end_round = rnd
            break

    sa, sb = scorecard(ca), scorecard(cb)
    if winner is None:
        if abs(sa["total"] - sb["total"]) <= DRAW_MARGIN:
            method = "Draw"
        else:
            winner = fa if sa["total"] > sb["total"] else fb
            method = "Decision"
    return BoutResult(a=fa, b=fb, winner=winner, method=method,
                      end_round=end_round, rounds_total=rounds_total,
                      score_a=sa, score_b=sb, log=log)

# ---------------------------------------------------------------- career

def find_opponent(f, week):
    """Deterministic stand-in for the matchmaking pool (D13, D53):
    same weight class, mined total within the half-to-double band,
    closest to the fighter's own mined total. Matching on the mined
    total (his established standing) rather than his declined current
    condition is deliberate: it is what makes decline visible, the
    same class of opponent he used to beat starts beating him.
    Prototype simplification: opponents always fight at full strength."""
    best = None
    seen = 0
    k = 0
    while seen < 25 and k < 400:
        o = generate_fighter(f"{f.seed}-opp-{week}-{k}")
        k += 1
        if o.wclass == f.wclass and 0.5 <= o.total / f.total <= 2.0:
            seen += 1
            if best is None or abs(o.total - f.total) < abs(best.total - f.total):
                best = o
    return best

def simulate_career(seed):
    f = generate_fighter(seed)
    rec = {"Prime": [0, 0, 0], "Decline": [0, 0, 0]}   # W, L, D by phase
    finishes = 0
    title_w = title_l = title_d = 0
    xp = 0                     # professional experience, D52 (cap +15 applied)
    log_lines = []
    for week in range(f.career_weeks):
        phase = phase_at(f, week)
        opp = find_opponent(f, week)
        if opp is None:
            continue
        is_title = (phase == "Prime" and week % 12 == 11
                    and f.tier in ("Contender", "Elite", "Generational"))
        res = simulate_bout(f, opp, f"career-{seed}-w{week}",
                            title=is_title, featured=is_title,
                            skill_bonus_a=min(15, xp),
                            base_a=effective_attrs(f, week), verbose=False)
        xp += 2 if is_title else 1
        if res.winner is None:
            rec[phase][2] += 1
            if is_title:
                title_d += 1
        elif res.winner is f:
            rec[phase][0] += 1
            if res.method != "Decision":
                finishes += 1
            if is_title:
                title_w += 1
        else:
            rec[phase][1] += 1
            if is_title:
                title_l += 1
    w = rec["Prime"][0] + rec["Decline"][0]
    l = rec["Prime"][1] + rec["Decline"][1]
    d = rec["Prime"][2] + rec["Decline"][2]
    end = effective_attrs(f, f.career_weeks)
    a = f.attrs
    log_lines.append(fighter_card(f))
    log_lines.append(f"CAREER: {w}-{l}-{d} ({finishes} wins by finish) over {f.career_weeks} weeks")
    for phase in ("Prime", "Decline"):
        pw, pl, pd = rec[phase]
        log_lines.append(f"  {phase}: {pw}-{pl}-{pd}")
    if title_w + title_l + title_d:
        log_lines.append(f"  Title fights: {title_w}-{title_l}-{title_d}")
    log_lines.append(f"  Pro experience at retirement: +{min(15, xp)} effective Skill (cap +15, D52)")
    log_lines.append(f"  Attributes at retirement (current condition, D23): "
                     f"POW {end['Power']:.0f} (was {a['Power']}), SPD {end['Speed']:.0f} (was {a['Speed']}), "
                     f"CHN {end['Chin']:.0f} (was {a['Chin']}), STA {end['Stamina']:.0f} (was {a['Stamina']}), "
                     f"SKL {end['Skill']:.0f} (was {a['Skill']})")
    log_lines.append(f"  Retired from professional fights after week {f.career_weeks}; "
                     f"still owned, exhibition and trade eligible (D22).")
    return "\n".join(log_lines)

def cmd_career(args):
    print(simulate_career(args.seed))

# ---------------------------------------------------------------- league

POINTS_FINISH, POINTS_DECISION, POINTS_DRAW = 14, 10, 4   # D41
RANK_WINDOW = 26                                          # D38, weeks

@dataclass
class LeagueFighter:
    f: Fighter
    born_week: int
    wins: int = 0
    losses: int = 0
    draws: int = 0
    finishes: int = 0
    xp: int = 0
    rating: float = 0.0        # D65 weighted rating, refreshed weekly
    ledger: list = field(default_factory=list)           # (week, points)
    priority: int = 0          # unmatched weeks, for next-week priority (D45)
    title_blocked: bool = False  # lost a title fight; needs a non-title win (D18)
    cooldown_until: int = -1     # also out of the title draw until this week (D64)
    transfer_blocked: bool = False  # must win under the new gym first (D38)
    gym: object = None
    retired: bool = False

    def age(self, week):
        return week - self.born_week

    def pro_fights(self):
        return self.wins + self.losses + self.draws

    def points(self, week):
        return sum(p for w, p in self.ledger if week - w < RANK_WINDOW)

    def record(self):
        return f"{self.wins}-{self.losses}-{self.draws}"

# D65: league rating weights, fitted by logistic regression on 6,000
# simulated bouts and rounded (rating_study.py). Speed pays twice in
# the engine (initiative and defense), Chin least in 3-round fights.
RATING_WEIGHTS = {"Power": 1.0, "Speed": 2.5, "Chin": 0.2,
                  "Stamina": 0.4, "Skill": 0.8}

def league_rating(lf, week):
    """D65: rating on current-condition attributes at the given week.
    Skill includes permanent pro XP; form and the weekly training
    bonus are excluded."""
    eff = dict(effective_attrs(lf.f, lf.age(week)))
    eff["Skill"] += min(15, lf.xp)
    return sum(RATING_WEIGHTS[a] * eff[a] for a in ATTRS)

def simulate_season(seed="genesis", weeks=52, wclass="Middle", roster_size=28,
                    prospects_per_week=2):
    roster, i = [], 0
    while len(roster) < roster_size:
        f = generate_fighter(f"{seed}-roster-{i}")
        i += 1
        if f.wclass == wclass:
            lf = LeagueFighter(f=f, born_week=0)
            lf.rating = league_rating(lf, 0)
            roster.append(lf)

    champion = None
    last_defense = -10
    reign = None               # [LeagueFighter, start_week, defenses]
    lineage = []
    events = []
    total_bouts = total_finishes = 0
    prospects_added = 0

    def close_reign(end_week):
        nonlocal reign
        if reign is not None:
            lineage.append((reign[0].f.name, reign[1], end_week, reign[2]))
            reign = None

    for week in range(weeks):
        wrng = rng_for("week", seed, week)

        # New prospects arrive (mined continuously in the real league;
        # D30 targets a prospect every 2-3 days per typical gym).
        if week > 0 and prospects_per_week > 0:
            added, j = 0, 0
            while added < prospects_per_week:
                f = generate_fighter(f"{seed}-prospect-{week}-{j}")
                j += 1
                if f.wclass == wclass:
                    lf = LeagueFighter(f=f, born_week=week)
                    lf.rating = league_rating(lf, week)
                    roster.append(lf)
                    added += 1
                    prospects_added += 1

        # Retirements (D22); a retiring champion vacates at once (D39).
        for lf in roster:
            if not lf.retired and lf.age(week) >= lf.f.career_weeks:
                lf.retired = True
                events.append(f"Week {week}: {lf.f.name} retires ({lf.record()})")
                if champion is lf:
                    champion = None
                    close_reign(week - 1)
                    events.append(f"Week {week}: belt vacated, champion retired")

        active = [lf for lf in roster if not lf.retired]
        for lf in active:                        # D65: price tonight's fighter
            lf.rating = league_rating(lf, week)
        ranked = sorted(active, key=lambda lf: (-lf.points(week), -lf.rating))
        rank_of = {lf.f.fid: idx + 1 for idx, lf in enumerate(ranked)}
        booked = set()
        pairs = []

        def eligible(exclude=None):
            out = []
            for lf in ranked:
                if (lf is exclude or lf.pro_fights() < 3 or lf.title_blocked
                        or week < lf.cooldown_until):
                    continue
                out.append(lf)
                if len(out) >= 5:
                    break
            return out

        # Belt first: inaugural or vacant title from week 4 (D20),
        # otherwise a defense at least every 2 weeks (D39).
        if champion is None and week >= 4:
            elig = eligible()
            if len(elig) >= 2:
                a, b = elig[0], wrng.choice(elig[1:5])
                pairs.append((a, b, True))
                booked.update({a.f.fid, b.f.fid})
        elif champion is not None and week - last_defense >= 2:
            chall = eligible(exclude=champion)
            if chall:
                b = wrng.choice(chall)
                pairs.append((champion, b, True))
                booked.update({champion.f.fid, b.f.fid})

        # The rest of the card: everyone matched gets a fight (D44).
        order = sorted([lf for lf in active if lf.f.fid not in booked],
                       key=lambda lf: (-lf.priority, wrng.random()))
        for lf in order:
            if lf.f.fid in booked:
                continue
            others = [o for o in active if o is not lf and o.f.fid not in booked]
            band, cands = 2.0, []
            while True:
                cands = [o for o in others
                         if lf.rating / band <= o.rating <= lf.rating * band]
                if len(cands) >= 5 or band > 8.0:
                    break
                band *= 1.5                     # D53: widen until 5 eligible
            if not cands:
                cands = others
            if cands:
                opp = wrng.choice(cands)
                booked.update({lf.f.fid, opp.f.fid})
                pairs.append((lf, opp, False))
                lf.priority = opp.priority = 0
            else:
                lf.priority += 1

        # Run the card.
        for la, lb, is_title in pairs:
            res = simulate_bout(
                la.f, lb.f, f"{seed}-w{week}-{la.f.fid}-{lb.f.fid}",
                title=is_title, featured=is_title,
                skill_bonus_a=min(15, la.xp), skill_bonus_b=min(15, lb.xp),
                base_a=effective_attrs(la.f, la.age(week)),
                base_b=effective_attrs(lb.f, lb.age(week)),
                verbose=False)
            total_bouts += 1
            la.xp += 2 if is_title else 1
            lb.xp += 2 if is_title else 1
            if res.winner is None:
                la.draws += 1
                lb.draws += 1
                la.ledger.append((week, POINTS_DRAW))
                lb.ledger.append((week, POINTS_DRAW))
                if is_title:
                    last_defense = week   # a drawn defense still counts as defended
            else:
                wl, ll = (la, lb) if res.winner is la.f else (lb, la)
                wl.wins += 1
                ll.losses += 1
                by_finish = res.method != "Decision"
                if by_finish:
                    wl.finishes += 1
                    total_finishes += 1
                pts = POINTS_FINISH if by_finish else POINTS_DECISION
                opp_rank = rank_of.get(ll.f.fid, 99)
                if opp_rank <= 5:
                    pts += 6                    # D41 quality bonus
                elif opp_rank <= 10:
                    pts += 3
                wl.ledger.append((week, pts))
                if not is_title:
                    wl.title_blocked = False
                if is_title:
                    last_defense = week
                    if champion is not None and ll is champion:
                        events.append(f"Week {week}: {wl.f.name} beats {ll.f.name} "
                                      f"({res.method}) and takes the belt")
                        close_reign(week - 1)
                        champion = wl
                        reign = [wl, week, 0]
                    elif champion is not None and wl is champion:
                        reign[2] += 1
                        events.append(f"Week {week}: {wl.f.name} defends the belt "
                                      f"against {ll.f.name} ({res.method}), defense {reign[2]}")
                    elif champion is None:
                        events.append(f"Week {week}: {wl.f.name} beats {ll.f.name} "
                                      f"({res.method}) and wins the vacant belt")
                        champion = wl
                        reign = [wl, week, 0]
                    if ll is not champion:
                        ll.title_blocked = True   # D18
                        ll.cooldown_until = week + 4   # D64

    close_reign(weeks - 1)
    out = [f"SEASON: {wclass} division, {weeks} weeks, seed {seed}",
           f"Roster: {roster_size} at genesis, {prospects_added} prospects added, "
           f"{total_bouts} bouts, {total_finishes} finishes "
           f"({total_finishes / max(1, total_bouts):.0%})",
           "", "EVENTS:"]
    out.extend("  " + e for e in events)
    out.append("")
    out.append("BELT LINEAGE:")
    for name, start, end, defenses in lineage:
        word = "defense" if defenses == 1 else "defenses"
        out.append(f"  {name}: weeks {start}-{end} ({end - start + 1} weeks, {defenses} {word})")
    out.append("")
    out.append(f"FINAL RANKINGS (rolling {RANK_WINDOW} week points):")
    final = sorted([lf for lf in roster if not lf.retired],
                   key=lambda lf: (-lf.points(weeks), -lf.rating))
    for idx, lf in enumerate(final[:10], 1):
        out.append(f"  {idx}. {lf.f.name} | {lf.points(weeks)} pts | {lf.record()} | "
                   f"rating {lf.rating:.0f} | {lf.f.tier} | "
                   f"{phase_at(lf.f, lf.age(weeks))}, age {lf.age(weeks)}w")
    if champion is not None:
        out.append(f"\nChampion at season end: {champion.f.name}")
    return "\n".join(out)

# ---------------------------------------------------------------- gyms and economy

BLOCKS_PER_WEEK = 5040            # 2 minute blocks (D32)
BLOCK_REWARD = 10.0               # SLUG per block (D32)
POOL_FEE = 0.01                   # official pool fee (D56)
MARKET_FEE = 0.05                 # marketplace commission (D37)
ROSTER_SPOTS = 10                 # D27

GYM_ADJ = ["Iron", "Canal", "Foundry", "Empire", "Granite", "Harbor", "Atlas",
           "Liberty", "Summit", "Victory", "Golden", "Union", "Premier", "Cinder"]
GYM_NOUN = ["Works", "Street", "House", "Club", "Stable", "Athletic", "Boxing"]
TIER_BASE_PRICE = {"Club": 20.0, "Professional": 60.0, "Contender": 150.0,
                   "Elite": 400.0, "Generational": 1200.0}
TIER_BUDGET = {name: (lo, hi) for name, _cum, lo, hi in TIERS}

@dataclass
class Gym:
    gid: int
    name: str
    devices: int
    slug: float = 0.0
    stable: list = field(default_factory=list)
    prospects: int = 0
    by_tier: dict = field(default_factory=dict)
    sales: int = 0
    buys: int = 0

    def roster(self):
        act = [lf for lf in self.stable if not lf.retired]
        act.sort(key=lambda lf: -lf.rating)
        return act[:ROSTER_SPOTS]

    def roster_points(self, week):
        return sum(lf.points(week) for lf in self.roster())

def price_for(lf, week):
    """Prototype placeholder pricing (the spec sets marketplace rules,
    not prices): tier base scaled by where the total sits in the tier,
    discounted as the fighter declines."""
    lo, hi = TIER_BUDGET[lf.f.tier]
    frac = (lf.f.total - lo) / max(1, hi - lo)
    price = TIER_BASE_PRICE[lf.f.tier] * (0.7 + 0.6 * frac)
    age = week - lf.born_week
    phase = phase_at(lf.f, age)
    if phase == "Decline":
        price *= 1.0 - 0.5 * ((age - lf.f.prime_weeks) / lf.f.decline_weeks)
    elif phase == "Retired":
        price *= 0.3
    return round(price, 1)

class Division:
    def __init__(self, wclass):
        self.wclass = wclass
        self.members = []        # every LeagueFighter in the class
        self.champion = None
        self.last_defense = -10
        self.reign = None        # [LeagueFighter, start_week, defenses]
        self.reigns = 0
        self.bouts = 0

def cmd_season(args):
    print(simulate_season(seed=args.seed, weeks=args.weeks,
                          wclass=args.wclass, roster_size=args.roster,
                          prospects_per_week=args.inflow))

def simulate_gyms(seed="gyms", weeks=26, gym_count=30):
    import bisect
    grng = rng_for("gyms", seed)
    gyms = []
    for gid in range(gym_count):
        r = grng.random()
        devices = 1 if r < 0.60 else 2 if r < 0.85 else grng.randint(3, 4) if r < 0.95 else grng.randint(5, 8)
        name = f"{GYM_ADJ[gid % len(GYM_ADJ)]} {GYM_NOUN[(gid // len(GYM_ADJ)) % len(GYM_NOUN)]} Gym"
        gyms.append(Gym(gid=gid, name=name, devices=devices))
    total_devices = sum(g.devices for g in gyms)
    cum_devices = []
    acc = 0
    for g in gyms:
        acc += g.devices
        cum_devices.append(acc)
    # D30 calibration: a 1-device gym should find a prospect about
    # every 2.5 days. It finds 720/total_devices blocks a day, so the
    # per-block prospect probability is 0.4 * total_devices / 720.
    prospect_p = 0.4 * total_devices / 720.0

    divisions = {name: Division(name) for name, _cap in WEIGHT_CLASSES}
    for g in gyms:  # onboarding tryout fighter (rookie, from the design)
        f = generate_fighter(f"{seed}-tryout-{g.gid}")
        lf = LeagueFighter(f=f, born_week=0)
        lf.rating = league_rating(lf, 0)
        lf.gym = g
        g.stable.append(lf)
        divisions[f.wclass].members.append(lf)

    events = []
    company_pool = company_market = 0.0
    market_stats = {"sales": 0, "volume": 0.0, "biggest": (0.0, "")}
    total_bouts = 0

    def resolve(la, lb, is_title, week, div, rank_of):
        nonlocal total_bouts
        res = simulate_bout(
            la.f, lb.f, f"{seed}-g{week}-{la.f.fid}-{lb.f.fid}",
            title=is_title, featured=is_title,
            skill_bonus_a=min(15, la.xp), skill_bonus_b=min(15, lb.xp),
            base_a=effective_attrs(la.f, la.age(week)),
            base_b=effective_attrs(lb.f, lb.age(week)), verbose=False)
        total_bouts += 1
        div.bouts += 1
        la.xp += 2 if is_title else 1
        lb.xp += 2 if is_title else 1
        if res.winner is None:
            la.draws += 1
            lb.draws += 1
            la.ledger.append((week, POINTS_DRAW))
            lb.ledger.append((week, POINTS_DRAW))
            if is_title:
                div.last_defense = week
            return
        wl, ll = (la, lb) if res.winner is la.f else (lb, la)
        wl.wins += 1
        ll.losses += 1
        if res.method != "Decision":
            wl.finishes += 1
        pts = POINTS_FINISH if res.method != "Decision" else POINTS_DECISION
        opp_rank = rank_of.get(ll.f.fid, 99)
        if opp_rank <= 5:
            pts += 6
        elif opp_rank <= 10:
            pts += 3
        wl.ledger.append((week, pts))
        if not is_title:
            wl.title_blocked = False
            wl.transfer_blocked = False
        if is_title:
            div.last_defense = week
            if div.champion is not None and ll is div.champion:
                events.append(f"Week {week} ({div.wclass}): {wl.f.name} ({wl.gym.name}) "
                              f"takes the belt from {ll.f.name} ({res.method})")
                div.champion = wl
                div.reign = [wl, week, 0]
                div.reigns += 1
            elif div.champion is not None and wl is div.champion:
                div.reign[2] += 1
            elif div.champion is None:
                events.append(f"Week {week} ({div.wclass}): {wl.f.name} ({wl.gym.name}) "
                              f"wins the vacant belt ({res.method})")
                div.champion = wl
                div.reign = [wl, week, 0]
                div.reigns += 1
            if ll is not div.champion:
                ll.title_blocked = True
                ll.cooldown_until = week + 4

    for week in range(weeks):
        wrng = rng_for("gymweek", seed, week)
        # Mining: prospects attach to found blocks; rewards are pooled
        # and split by devices, minus the 1% pool fee (D55-D58).
        for h in range(BLOCKS_PER_WEEK):
            finder = gyms[bisect.bisect_left(cum_devices, wrng.random() * total_devices)]
            if wrng.random() < prospect_p:
                f = generate_fighter(f"{seed}-pros-{week}-{h}")
                lf = LeagueFighter(f=f, born_week=week)
                lf.rating = league_rating(lf, week)
                lf.gym = finder
                finder.stable.append(lf)
                finder.prospects += 1
                finder.by_tier[f.tier] = finder.by_tier.get(f.tier, 0) + 1
                divisions[f.wclass].members.append(lf)
        week_reward = BLOCKS_PER_WEEK * BLOCK_REWARD
        for g in gyms:
            g.slug += week_reward * (g.devices / total_devices) * (1.0 - POOL_FEE)
        company_pool += week_reward * POOL_FEE

        # Retirements; a retiring champion vacates (D22, D39).
        for div in divisions.values():
            for lf in div.members:
                if not lf.retired and lf.age(week) >= lf.f.career_weeks:
                    lf.retired = True
                    if div.champion is lf:
                        div.champion = None
                        div.reign = None
                        events.append(f"Week {week} ({div.wclass}): belt vacated, champion retired")

        # D65: refresh ratings on current condition before any use.
        for div in divisions.values():
            for lf in div.members:
                if not lf.retired:
                    lf.rating = league_rating(lf, week)

        # Rosters and Saturday entries: best rostered fighter per class (D27-D29).
        entered = {name: [] for name, _cap in WEIGHT_CLASSES}
        for g in gyms:
            best = {}
            for lf in g.roster():
                cur = best.get(lf.f.wclass)
                if cur is None or lf.rating > cur.rating:
                    best[lf.f.wclass] = lf
            for cls, lf in best.items():
                entered[cls].append(lf)

        # Run each division's card.
        for cls, div in divisions.items():
            active = [lf for lf in div.members if not lf.retired]
            if not active:
                continue
            ranked = sorted(active, key=lambda lf: (-lf.points(week), -lf.rating))
            rank_of = {lf.f.fid: idx + 1 for idx, lf in enumerate(ranked)}
            booked = set()
            pairs = []

            def eligible(exclude=None):
                out = []
                for lf in ranked:
                    if (lf is exclude or lf.pro_fights() < 3 or lf.title_blocked
                            or lf.transfer_blocked or week < lf.cooldown_until):
                        continue
                    out.append(lf)
                    if len(out) >= 5:
                        break
                return out

            if div.champion is None and week >= 4:
                elig = eligible()
                if len(elig) >= 2:
                    a, b = elig[0], wrng.choice(elig[1:5])
                    pairs.append((a, b, True))
                    booked.update({a.f.fid, b.f.fid})
            elif div.champion is not None and week - div.last_defense >= 2:
                chall = eligible(exclude=div.champion)
                if chall:
                    b = wrng.choice(chall)
                    pairs.append((div.champion, b, True))
                    booked.update({div.champion.f.fid, b.f.fid})

            pool = [lf for lf in entered[cls] if lf.f.fid not in booked]
            order = sorted(pool, key=lambda lf: (-lf.priority, wrng.random()))
            for lf in order:
                if lf.f.fid in booked:
                    continue
                others = [o for o in pool if o is not lf and o.f.fid not in booked]
                band, cands = 2.0, []
                while True:
                    cands = [o for o in others
                             if lf.rating / band <= o.rating <= lf.rating * band]
                    if len(cands) >= 5 or band > 8.0:
                        break
                    band *= 1.5
                if not cands:
                    cands = others
                if cands:
                    opp = wrng.choice(cands)
                    booked.update({lf.f.fid, opp.f.fid})
                    pairs.append((lf, opp, False))
                    lf.priority = opp.priority = 0
                else:
                    lf.priority += 1
            for la, lb, is_title in pairs:
                resolve(la, lb, is_title, week, div, rank_of)

        # Monthly market day (D37, D60): gyms list non-rostered fighters,
        # other gyms buy for SLUG, 5% commission to the company.
        if week % 4 == 3:
            listings = []
            for g in gyms:
                rostered = {id(x.f) for x in g.roster()}
                spares = [lf for lf in g.stable
                          if not lf.retired and id(lf.f) not in rostered
                          and divisions[lf.f.wclass].champion is not lf]
                spares.sort(key=lambda lf: -lf.rating)
                for lf in spares[:3]:
                    listings.append([g, lf, price_for(lf, week), False])
            for buyer in gyms:
                broster = buyer.roster()
                by_class = {}
                for lf in broster:
                    by_class.setdefault(lf.f.wclass, []).append(lf)
                for lst in listings:
                    seller, lf, price, sold = lst
                    if sold or seller is buyer:
                        continue
                    if buyer.slug < price or price > 0.35 * buyer.slug + 25.0:
                        continue
                    cur = by_class.get(lf.f.wclass)
                    need = cur is None or lf.rating > min(x.rating for x in cur) + 30.0
                    if not need:
                        continue
                    buyer.slug -= price
                    seller.slug += price * (1.0 - MARKET_FEE)
                    company_market += price * MARKET_FEE
                    seller.stable.remove(lf)
                    buyer.stable.append(lf)
                    lf.gym = buyer
                    lf.transfer_blocked = True
                    seller.sales += 1
                    buyer.buys += 1
                    lst[3] = True
                    market_stats["sales"] += 1
                    market_stats["volume"] += price
                    if price > market_stats["biggest"][0]:
                        market_stats["biggest"] = (price, f"{lf.f.name} ({lf.f.tier}), "
                                                        f"{seller.name} to {buyer.name}")
                    by_class.setdefault(lf.f.wclass, []).append(lf)

    one_dev = [g for g in gyms if g.devices == 1]
    tot_prospects = sum(g.prospects for g in gyms)
    tier_totals = {}
    for g in gyms:
        for t, c in g.by_tier.items():
            tier_totals[t] = tier_totals.get(t, 0) + c
    elite_1dev = sum(g.by_tier.get("Elite", 0) for g in one_dev)
    gen_1dev = sum(g.by_tier.get("Generational", 0) for g in one_dev)
    out = [f"GYM ECONOMY: {gym_count} gyms, {total_devices} devices, {weeks} weeks",
           f"Chain: {weeks * BLOCKS_PER_WEEK} blocks, {weeks * BLOCKS_PER_WEEK * BLOCK_REWARD:,.0f} SLUG issued "
           f"(of the 21,000,000 cap)",
           f"Company revenue: pool fees {company_pool:,.0f} SLUG, "
           f"marketplace commissions {company_market:,.0f} SLUG",
           "",
           f"Prospects found: {tot_prospects} total. Tier mix: "
           + ", ".join(f"{t} {tier_totals.get(t, 0) / max(1, tot_prospects):.0%}"
                       for t, _c, _l, _h in TIERS),
           f"Calibration (D30): {len(one_dev)} one-device gyms found "
           f"{sum(g.prospects for g in one_dev) / max(1, len(one_dev)) / weeks:.2f} prospects/week each "
           f"(target about 2.8, one per 2.5 days); "
           f"an Elite every {weeks * len(one_dev) / max(1, elite_1dev):.0f} gym-weeks (target 6-8), "
           f"a Generational every {weeks * len(one_dev) / max(1, gen_1dev):.0f} gym-weeks (target about 36)",
           "",
           "CHAMPIONS AT END:"]
    for cls, div in divisions.items():
        champ = div.champion
        out.append(f"  {cls}: {champ.f.name} ({champ.gym.name})" if champ
                   else f"  {cls}: vacant")
        rword = "reign" if div.reigns == 1 else "reigns"
        out.append(f"    {div.reigns} {rword}, {div.bouts} bouts")
    out.append("")
    out.append(f"MARKET: {market_stats['sales']} sales, {market_stats['volume']:,.0f} SLUG volume, "
               f"average {market_stats['volume'] / max(1, market_stats['sales']):,.0f} SLUG")
    if market_stats["biggest"][0]:
        out.append(f"  Biggest sale: {market_stats['biggest'][0]:,.0f} SLUG, {market_stats['biggest'][1]}")
    out.append("")
    out.append("TOP GYMS BY SLUG BALANCE:")
    for g in sorted(gyms, key=lambda g: -g.slug)[:5]:
        out.append(f"  {g.name}: {g.slug:,.0f} SLUG, {g.devices} devices, "
                   f"{len(g.stable)} fighters owned, {g.sales} sold, {g.buys} bought")
    out.append("TOP GYMS BY ROSTER POINTS:")
    for g in sorted(gyms, key=lambda g: -g.roster_points(weeks))[:5]:
        out.append(f"  {g.name}: {g.roster_points(weeks)} pts")
    out.append("")
    out.append("TITLE EVENTS:")
    out.extend("  " + e for e in events)
    return "\n".join(out)

def cmd_gyms(args):
    print(simulate_gyms(seed=args.seed, weeks=args.weeks, gym_count=args.gyms))

# ---------------------------------------------------------------- reports

def bout_report(res, show_log=True):
    out = []
    out.append(fighter_card(res.a))
    out.append(fighter_card(res.b))
    if show_log:
        out.extend(res.log)
    if res.winner is None:
        out.append(f"RESULT: Draw after {res.end_round} rounds")
    else:
        out.append(f"RESULT: {res.winner.name} wins by {res.method}, round {res.end_round} of {res.rounds_total}")
    out.append(f"Scorecard (published, whole fight):")
    for f, s in ((res.a, res.score_a), (res.b, res.score_b)):
        out.append(f"  {f.name}: total {s['total']} = damage {s['damage']} + knockdowns {s['knockdowns']} "
                   f"+ accuracy {s['accuracy']} + volume {s['volume']} + control {s['control']}")
    return "\n".join(out)

def cmd_bout(args):
    fa, fb = generate_fighter(args.seed_a), generate_fighter(args.seed_b)
    if fa.wclass != fb.wclass:
        print(f"Note: cross-class pairing ({fa.wclass} vs {fb.wclass}); the league would not book this.")
    res = simulate_bout(fa, fb, args.bout_seed, title=args.title, featured=args.title)
    print(bout_report(res))

def cmd_card(args):
    prospects = {}
    for i in range(40):
        f = generate_fighter(f"{args.base}-{i}")
        prospects.setdefault(f.wclass, []).append(f)
    classes = [c for c, fs in prospects.items() if len(fs) >= 2]
    if len(classes) < 4:
        print("Not enough matched pairs in the prospect pool.")
        return
    print(f"THE MINE EVENT (prototype card) | lock block seed: {args.bout_seed}")
    print("=" * 72)
    for i, cls in enumerate(classes[:3]):
        fa, fb = prospects[cls][0], prospects[cls][1]
        print(f"\nBout {i + 1} ({cls}, 3 rounds)")
        print("-" * 72)
        res = simulate_bout(fa, fb, f"{args.bout_seed}:bout{i + 1}")
        print(bout_report(res))
    cls = classes[3]
    fa, fb = prospects[cls][0], prospects[cls][1]
    print(f"\nMAIN EVENT ({cls} title, 5 rounds, featured seeding)")
    print("-" * 72)
    res = simulate_bout(fa, fb, f"{args.bout_seed}:main", title=True, featured=True)
    print(bout_report(res))

def cmd_balance(args):
    n = args.n
    finishes = decisions = draws = 0
    fav_wins = fav_fights = 0
    ko = tko = 0
    rounds_sum = 0
    for i in range(n):
        fa = generate_fighter(f"bal-{args.seed}-{i}-a")
        fb = generate_fighter(f"bal-{args.seed}-{i}-b")
        res = simulate_bout(fa, fb, f"bal-bout-{args.seed}-{i}", verbose=False)
        rounds_sum += res.end_round
        if res.method == "Draw":
            draws += 1
            continue
        if res.method == "Decision":
            decisions += 1
        else:
            finishes += 1
            if res.method == "KO":
                ko += 1
            else:
                tko += 1
        if fa.total != fb.total:
            fav_fights += 1
            fav = fa if fa.total > fb.total else fb
            if res.winner is fav:
                fav_wins += 1
    print(f"Bouts: {n}")
    print(f"  Finishes:  {finishes} ({finishes / n:.1%})  KO {ko}, TKO {tko}")
    print(f"  Decisions: {decisions} ({decisions / n:.1%})")
    print(f"  Draws:     {draws} ({draws / n:.1%})")
    print(f"  Higher-rated fighter won {fav_wins}/{fav_fights} non-draw bouts ({fav_wins / max(1, fav_fights):.1%})")
    print(f"  Average length: {rounds_sum / n:.2f} rounds")

def main():
    p = argparse.ArgumentParser(description="SLUG fight simulation prototype")
    sub = p.add_subparsers(dest="cmd", required=True)

    pb = sub.add_parser("bout")
    pb.add_argument("seed_a")
    pb.add_argument("seed_b")
    pb.add_argument("--title", action="store_true")
    pb.add_argument("--bout-seed", default="lock-block-demo")
    pb.set_defaults(fn=cmd_bout)

    pc = sub.add_parser("card")
    pc.add_argument("--base", default="genesis-prospect")
    pc.add_argument("--bout-seed", default="saturday-lock-0001")
    pc.set_defaults(fn=cmd_card)

    pn = sub.add_parser("balance")
    pn.add_argument("n", nargs="?", type=int, default=300)
    pn.add_argument("--seed", default="v1")
    pn.set_defaults(fn=cmd_balance)

    pr = sub.add_parser("career")
    pr.add_argument("seed")
    pr.set_defaults(fn=cmd_career)

    ps = sub.add_parser("season")
    ps.add_argument("--seed", default="genesis")
    ps.add_argument("--weeks", type=int, default=52)
    ps.add_argument("--wclass", default="Middle")
    ps.add_argument("--roster", type=int, default=28)
    ps.add_argument("--inflow", type=int, default=2,
                    help="new prospects per week in this weight class")
    ps.set_defaults(fn=cmd_season)

    pg = sub.add_parser("gyms")
    pg.add_argument("--seed", default="gyms")
    pg.add_argument("--weeks", type=int, default=26)
    pg.add_argument("--gyms", type=int, default=30)
    pg.set_defaults(fn=cmd_gyms)

    args = p.parse_args()
    args.fn(args)

if __name__ == "__main__":
    sys.exit(main())
