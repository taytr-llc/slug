#!/usr/bin/env python3
"""Deterministic fighter portraits, generated from the chain.

A portrait is a pure function of the fighter's id (his mined hash)
plus two pieces of his DNA that are also hash-derived: weight class
(build) and tier (frame ring). Any client anywhere renders the same
portrait from the same fighter, with no image storage and no artist.
No user customization, same rule as names (D7, D9).

Trait tables here are the v1 portrait generation tables (spec
section 17 build-time item). Output is a self-contained SVG string.

Run directly to render a contact sheet of sample fighters:
    python3 portraits.py
"""

import hashlib

SKIN = [("#f2c79b", "#d9a86f"), ("#e0ac7e", "#c68d55"),
        ("#c6885a", "#a06a3e"), ("#a06638", "#7d4c26"),
        ("#7a4a24", "#5c3517"), ("#54331a", "#3c2311")]
HAIR_COLORS = ["#191512", "#2e2019", "#5a3a22", "#8a6134",
               "#b5b0a8", "#d8d3c8"]
BG = ["#23262b", "#26232a", "#222a2c", "#2a2622",
      "#252a24", "#2c2426", "#242832", "#28251f"]
TANK = ["#31435c", "#5c3131", "#31523f", "#4a315c",
        "#59503a", "#2e4a52", "#623f27", "#3a3f4a"]
TIER_RING = {"Club": "#8b93a1", "Professional": "#7fa6c9",
             "Contender": "#b48ce0", "Elite": "#d7dde6",
             "Generational": "#e8b64c"}
BUILD = {"Light": 54, "Welter": 59, "Middle": 64,
         "Light Heavy": 69, "Heavy": 74}


def _traits(fid):
    d = hashlib.sha256(fid.encode()).digest()
    return {
        "skin": d[0] % len(SKIN),
        "hair_style": d[1] % 8,
        "hair_color": d[2] % len(HAIR_COLORS),
        "facial": d[3] % 5,        # none, stubble, mustache, goatee, beard
        "shape": d[4] % 4,         # round, square, long, wide
        "mouth": d[5] % 4,         # flat, smirk, grin, grimace
        "scar": d[6] % 100,        # < 35 gets a scar, position from value
        "brow": d[7] % 3,
        "bg": d[8] % len(BG),
        "tank": d[9] % len(TANK),
        "jitter": (d[10] % 13) - 6,
    }


def _head_path(cx, top, side, bottom, jaw):
    return (f"M {cx},{top} "
            f"C {cx + side * 0.7},{top} {cx + side},{top + (bottom - top) * 0.35} "
            f"{cx + side},{(top + bottom) / 2} "
            f"C {cx + side},{bottom - (bottom - top) * 0.28} {cx + jaw},{bottom} "
            f"{cx},{bottom} "
            f"C {cx - jaw},{bottom} {cx - side},{bottom - (bottom - top) * 0.28} "
            f"{cx - side},{(top + bottom) / 2} "
            f"C {cx - side},{top + (bottom - top) * 0.35} {cx - side * 0.7},{top} "
            f"{cx},{top} Z")


def portrait_svg(fighter):
    t = _traits(fighter["id"])
    skin, skin_shadow = SKIN[t["skin"]]
    hair = HAIR_COLORS[t["hair_color"]]
    cx = 100
    shapes = [  # top, side, bottom, jaw
        (56, 31, 130, 20),   # round
        (58, 32, 131, 27),   # square
        (50, 27, 136, 16),   # long
        (58, 35, 128, 24),   # wide
    ]
    top, side, bottom, jaw = shapes[t["shape"]]
    sh = BUILD.get(fighter.get("wclass"), 64) + round(t["jitter"] * 0.5)
    P = []
    P.append(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200">')
    P.append(f'<rect width="200" height="200" fill="{BG[t["bg"]]}"/>')
    P.append('<ellipse cx="100" cy="86" rx="92" ry="82" fill="#ffffff" opacity="0.05"/>')
    # neck (short, tucked under the chin)
    P.append(f'<rect x="{cx - 12}" y="110" width="24" height="30" rx="6" fill="{skin_shadow}"/>')
    # upper body as ONE skin silhouette: neck, sloping shoulders,
    # deltoid bulge, and upper arms descending outside the ribcage,
    # all cropped by the bottom of the frame. One path, no seams,
    # so the arms can never detach from the shoulders.
    P.append(f'<path d="M {cx - 17},132 '
             f'C {cx - 36},137 {cx - sh - 10},140 {cx - sh - 13},154 '
             f'C {cx - sh - 15},168 {cx - sh - 12},184 {cx - sh - 11},200 '
             f'L {cx + sh + 11},200 '
             f'C {cx + sh + 12},184 {cx + sh + 15},168 {cx + sh + 13},154 '
             f'C {cx + sh + 10},140 {cx + 36},137 {cx + 17},132 '
             f'Q {cx},142 {cx - 17},132 Z" fill="{skin}"/>')
    # separate the arms from the torso: a soft shadow plane on each
    # arm, an armpit crease line down the inner arm, and a deltoid
    # cut where the shoulder muscle ends
    for sgn in (-1, 1):
        P.append(f'<path d="M {cx + sgn * (sh - 8)},154 '
                 f'C {cx + sgn * (sh + 3)},168 {cx + sgn * (sh - 1)},186 {cx + sgn * sh},200 '
                 f'L {cx + sgn * (sh + 11)},200 '
                 f'C {cx + sgn * (sh + 12)},184 {cx + sgn * (sh + 15)},168 {cx + sgn * (sh + 13)},154 '
                 f'C {cx + sgn * (sh + 4)},146 {cx + sgn * (sh - 2)},148 {cx + sgn * (sh - 8)},154 Z" '
                 f'fill="{skin_shadow}" opacity="0.12"/>')
        P.append(f'<path d="M {cx + sgn * (sh - 8)},154 '
                 f'C {cx + sgn * (sh + 3)},168 {cx + sgn * (sh - 1)},184 {cx + sgn * (sh - 2)},200" '
                 f'fill="none" stroke="{skin_shadow}" stroke-width="2.2" stroke-linecap="round" opacity="0.55"/>')
        P.append(f'<path d="M {cx + sgn * (sh + 9)},144 '
                 f'C {cx + sgn * (sh + 2)},152 {cx + sgn * (sh - 3)},158 {cx + sgn * (sh - 7)},163" '
                 f'fill="none" stroke="{skin_shadow}" stroke-width="2" stroke-linecap="round" opacity="0.4"/>')
    # shirtless, like a fighter at a weigh-in: chest definition in
    # the skin shadow tone, collarbones, and a center line. Every
    # chest measurement scales with the torso width (sh), so a
    # heavyweight's pecs sit wide on a wide chest and a light
    # fighter's stay narrow, instead of one fixed chest on all.
    collar_in, collar_out = round(sh * 0.22), round(sh * 0.46)
    pec_out, pec_mid = round(sh * 0.40), round(sh * 0.20)
    pec_dip = round(168 + sh * 0.13)
    nip_x = round(sh * 0.26)
    P.append(f'<path d="M {cx - collar_in},143 L {cx - collar_out},147" stroke="{skin_shadow}" stroke-width="2" stroke-linecap="round" opacity="0.35"/>')
    P.append(f'<path d="M {cx + collar_in},143 L {cx + collar_out},147" stroke="{skin_shadow}" stroke-width="2" stroke-linecap="round" opacity="0.35"/>')
    P.append(f'<path d="M {cx - pec_out},167 Q {cx - pec_mid},{pec_dip} {cx - 2},168" fill="none" stroke="{skin_shadow}" stroke-width="2" stroke-linecap="round" opacity="0.45"/>')
    P.append(f'<path d="M {cx + 2},168 Q {cx + pec_mid},{pec_dip} {cx + pec_out},167" fill="none" stroke="{skin_shadow}" stroke-width="2" stroke-linecap="round" opacity="0.45"/>')
    P.append(f'<path d="M {cx},150 L {cx},{150 + round(sh * 0.20)}" stroke="{skin_shadow}" stroke-width="1.6" opacity="0.25"/>')
    for sgn in (-1, 1):
        P.append(f'<circle cx="{cx + sgn * nip_x}" cy="171" r="1.7" fill="{skin_shadow}" opacity="0.5"/>')
    # ears
    P.append(f'<ellipse cx="{cx - side}" cy="97" rx="5" ry="8" fill="{skin_shadow}"/>')
    P.append(f'<ellipse cx="{cx + side}" cy="97" rx="5" ry="8" fill="{skin_shadow}"/>')
    # head
    P.append(f'<path d="{_head_path(cx, top, side, bottom, jaw)}" fill="{skin}"/>')
    # facial hair under the mouth layer
    jaw_y = bottom - 2
    if t["facial"] == 1:  # stubble
        P.append(f'<path d="M {cx - jaw - 4},{jaw_y - 26} C {cx - jaw},{jaw_y} '
                 f'{cx + jaw},{jaw_y} {cx + jaw + 4},{jaw_y - 26} '
                 f'C {cx + 14},{jaw_y - 16} {cx - 14},{jaw_y - 16} '
                 f'{cx - jaw - 4},{jaw_y - 26} Z" fill="{hair}" opacity="0.30"/>')
    if t["facial"] == 4:  # full beard
        P.append(f'<path d="M {cx - side + 3},94 C {cx - side + 1},{jaw_y - 8} '
                 f'{cx - jaw},{jaw_y + 3} {cx},{jaw_y + 3} '
                 f'C {cx + jaw},{jaw_y + 3} {cx + side - 1},{jaw_y - 8} '
                 f'{cx + side - 3},94 C {cx + 15},110 {cx - 15},110 '
                 f'{cx - side + 3},94 Z" fill="{hair}"/>')
    if t["facial"] in (2, 3):  # mustache (+ goatee chin)
        P.append(f'<path d="M {cx - 13},111 C {cx - 6},107 {cx - 2},110 {cx},112 '
                 f'C {cx + 2},110 {cx + 6},107 {cx + 13},111 '
                 f'C {cx + 7},116 {cx - 7},116 {cx - 13},111 Z" fill="{hair}"/>')
        if t["facial"] == 3:
            P.append(f'<ellipse cx="{cx}" cy="124" rx="8" ry="6" fill="{hair}"/>')
    # hair
    hs = t["hair_style"]
    if hs == 0:  # bald shine
        P.append(f'<ellipse cx="{cx - side * 0.35}" cy="{top + 9}" rx="10" ry="5" '
                 f'fill="#ffffff" opacity="0.10" transform="rotate(-18 {cx - side * 0.35} {top + 9})"/>')
    elif hs == 1:  # buzz
        P.append(f'<path d="{_head_path(cx, top - 1, side, 84, side - 4)}" fill="{hair}" opacity="0.92"/>')
    elif hs == 2:  # crop, straight fringe
        P.append(f'<path d="M {cx - side},92 C {cx - side},{top + 2} {cx - side * 0.5},{top - 3} '
                 f'{cx},{top - 3} C {cx + side * 0.5},{top - 3} {cx + side},{top + 2} {cx + side},92 '
                 f'L {cx + side},80 L {cx - side},80 Z" fill="{hair}"/>')
    elif hs == 3:  # mohawk
        P.append(f'<path d="M {cx - 7},90 C {cx - 11},{top - 6} {cx - 5},{top - 22} {cx},{top - 24} '
                 f'C {cx + 5},{top - 22} {cx + 11},{top - 6} {cx + 7},90 Z" fill="{hair}"/>')
    elif hs == 4:  # swept long top
        P.append(f'<path d="M {cx - side},94 C {cx - side - 2},{top - 2} {cx - side * 0.4},{top - 8} '
                 f'{cx + 6},{top - 6} C {cx + side * 0.8},{top - 4} {cx + side + 2},{top + 8} '
                 f'{cx + side},88 C {cx + side * 0.4},{top + 16} {cx - side * 0.5},{top + 14} '
                 f'{cx - side},94 Z" fill="{hair}"/>')
    elif hs == 5:  # curly
        import math
        for k in range(6):
            ang = math.pi * (0.08 + 0.84 * k / 5)
            hx = cx - math.cos(ang) * (side - 3)
            hy = (top + 16) - math.sin(ang) * 13
            P.append(f'<circle cx="{hx:.0f}" cy="{hy:.0f}" r="8.5" fill="{hair}"/>')
        P.append(f'<path d="{_head_path(cx, top, side - 2, 76, side - 6)}" fill="{hair}"/>')
    elif hs == 6:  # flat top
        P.append(f'<path d="M {cx - side + 4},84 L {cx - side + 2},{top + 2} '
                 f'L {cx + side - 2},{top + 2} L {cx + side - 4},84 '
                 f'C {cx + side * 0.4},{top + 14} {cx - side * 0.4},{top + 14} '
                 f'{cx - side + 4},84 Z" fill="{hair}"/>')
    else:  # fade, low hairline
        P.append(f'<path d="{_head_path(cx, top - 1, side, 80, side - 3)}" fill="{hair}"/>')
        P.append(f'<path d="{_head_path(cx, top + 3, side - 3, 84, side - 6)}" fill="{skin}"/>')
        P.append(f'<path d="{_head_path(cx, top + 3, side - 3, 80, side - 6)}" fill="{hair}" opacity="0.55"/>')
    # brows
    brow_y, brow_c = 83, hair
    if t["brow"] == 0:
        P.append(f'<path d="M {cx - 17},{brow_y} L {cx - 5},{brow_y}" stroke="{brow_c}" stroke-width="3.4" stroke-linecap="round"/>')
        P.append(f'<path d="M {cx + 5},{brow_y} L {cx + 17},{brow_y}" stroke="{brow_c}" stroke-width="3.4" stroke-linecap="round"/>')
    elif t["brow"] == 1:  # angled in
        P.append(f'<path d="M {cx - 17},{brow_y - 2} L {cx - 5},{brow_y + 2}" stroke="{brow_c}" stroke-width="3.4" stroke-linecap="round"/>')
        P.append(f'<path d="M {cx + 17},{brow_y - 2} L {cx + 5},{brow_y + 2}" stroke="{brow_c}" stroke-width="3.4" stroke-linecap="round"/>')
    else:  # arched
        P.append(f'<path d="M {cx - 17},{brow_y + 1} Q {cx - 11},{brow_y - 4} {cx - 5},{brow_y + 1}" stroke="{brow_c}" stroke-width="3.4" fill="none" stroke-linecap="round"/>')
        P.append(f'<path d="M {cx + 5},{brow_y + 1} Q {cx + 11},{brow_y - 4} {cx + 17},{brow_y + 1}" stroke="{brow_c}" stroke-width="3.4" fill="none" stroke-linecap="round"/>')
    # eyes
    for ex in (cx - 11, cx + 11):
        P.append(f'<ellipse cx="{ex}" cy="93" rx="5.4" ry="3.4" fill="#ece5d8"/>')
        P.append(f'<circle cx="{ex}" cy="93.4" r="1.9" fill="#1c1712"/>')
        P.append(f'<circle cx="{ex + 0.7}" cy="92.6" r="0.6" fill="#ffffff"/>')
    # nose
    P.append(f'<path d="M {cx},99 L {cx - 3},108 Q {cx},110.5 {cx + 3},108" fill="none" '
             f'stroke="{skin_shadow}" stroke-width="1.8" stroke-linecap="round"/>')
    # mouth
    my = 117
    if t["facial"] == 4:
        my = 115
    if t["mouth"] == 0:
        P.append(f'<path d="M {cx - 8},{my} L {cx + 8},{my}" stroke="#6e4630" stroke-width="2.4" stroke-linecap="round"/>')
    elif t["mouth"] == 1:  # smirk
        P.append(f'<path d="M {cx - 8},{my + 1} Q {cx + 2},{my + 2} {cx + 9},{my - 2}" stroke="#6e4630" stroke-width="2.4" fill="none" stroke-linecap="round"/>')
    elif t["mouth"] == 2:  # grin
        P.append(f'<rect x="{cx - 9}" y="{my - 3}" width="18" height="7" rx="3" fill="#4a2c1c"/>')
        P.append(f'<rect x="{cx - 9}" y="{my - 3}" width="18" height="3" rx="1.5" fill="#e8e2d4"/>')
    else:  # grimace, teeth
        P.append(f'<rect x="{cx - 10}" y="{my - 3}" width="20" height="6.5" rx="2" fill="#e8e2d4" stroke="#6e4630" stroke-width="1.2"/>')
        for gx in (cx - 3.5, cx + 3.5):
            P.append(f'<path d="M {gx},{my - 3} L {gx},{my + 3.5}" stroke="#6e4630" stroke-width="1"/>')
    # scar
    if t["scar"] < 35:
        if t["scar"] % 2 == 0:  # through the left brow
            P.append(f'<path d="M {cx - 18},79 L {cx - 7},86" stroke="#e8c9a0" stroke-width="1.7" opacity="0.85" stroke-linecap="round"/>')
        else:  # cheek
            P.append(f'<path d="M {cx + 13},103 L {cx + 18},111" stroke="#e8c9a0" stroke-width="1.7" opacity="0.85" stroke-linecap="round"/>')
    # tier frame
    ring = TIER_RING.get(fighter.get("tier"), "#8b93a1")
    P.append(f'<rect x="2.5" y="2.5" width="195" height="195" rx="12" fill="none" stroke="{ring}" stroke-width="3"/>')
    P.append('</svg>')
    return "".join(P)


if __name__ == "__main__":
    import json
    fighters = []
    ev = json.load(open("mine_event_trim.json"))
    for div in ev["divisions"].values():
        fighters.extend(div["fighters"].values())
    gm = json.load(open("gym_market.json"))
    fighters.extend(gm["gym_view"]["roster"])
    fighters.extend(gm["market"]["listings"])
    seen, uniq = set(), []
    for f in fighters:
        if f["id"] not in seen:
            seen.add(f["id"])
            uniq.append(f)
    cells = "".join(
        f'<figure>{portrait_svg(f)}<figcaption>{f["name"]}<br>'
        f'{f["tier"]} · {f["wclass"]}</figcaption></figure>'
        for f in uniq[:24])
    html = ('<html><head><style>body{background:#14161a;color:#ddd;'
            'font-family:sans-serif}figure{display:inline-block;margin:8px;'
            'text-align:center;font-size:12px}svg{width:150px;height:150px}'
            '</style></head><body>' + cells + '</body></html>')
    open("portrait_sheet.html", "w").write(html)
    try:
        import cairosvg
        from PIL import Image
        import io
        imgs = []
        for f in uniq[:24]:
            png = cairosvg.svg2png(bytestring=portrait_svg(f).encode(),
                                   output_width=150, output_height=150)
            imgs.append(Image.open(io.BytesIO(png)))
        cols = 6
        rows = (len(imgs) + cols - 1) // cols
        sheet = Image.new("RGB", (cols * 150, rows * 150), (20, 22, 26))
        for i, im in enumerate(imgs):
            sheet.paste(im, ((i % cols) * 150, (i // cols) * 150))
        sheet.save("portrait_sheet.png")
        print(f"sheet: {len(imgs)} portraits -> portrait_sheet.png")
    except Exception as e:
        print("png sheet failed:", e)
