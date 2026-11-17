// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterportrait

func face(d *document, style int, c, s, b, j int64, hair string) {
	J := b - 2
	var p path
	switch style {
	case 0:
		return
	case 1:
		p.cmd("M", u(c-j-4), u(J-26))
		p.cmd("C", u(c-j), u(J), u(c+j), u(J), u(c+j+4), u(J-26))
		p.cmd("C", u(c+14), u(J-16), u(c-14), u(J-16), u(c-j-4), u(J-26))
		p.close()
		d.path(p.String(), hair, a("opacity", "0.3"))
	case 2, 3:
		p.cmd("M", u(c-13), u(111))
		p.cmd("C", u(c-6), u(107), u(c-2), u(110), u(c), u(112))
		p.cmd("C", u(c+2), u(110), u(c+6), u(107), u(c+13), u(111))
		p.cmd("C", u(c+7), u(116), u(c-7), u(116), u(c-13), u(111))
		p.close()
		d.path(p.String(), hair)
		if style == 3 {
			d.element("ellipse", whole("cx", c), whole("cy", 124), whole("rx", 8), whole("ry", 6), a("fill", hair))
		}
	case 4:
		p.cmd("M", u(c-s+3), u(94))
		p.cmd("C", u(c-s+1), u(J-8), u(c-j), u(J+3), u(c), u(J+3))
		p.cmd("C", u(c+j), u(J+3), u(c+s-1), u(J-8), u(c+s-3), u(94))
		p.cmd("C", u(c+15), u(110), u(c-15), u(110), u(c-s+3), u(94))
		p.close()
		d.path(p.String(), hair)
	default:
		d.err = ErrPortrait
	}
}

// Literal centers frozen by the contract's independent high-precision
// trigonometric calculation. A value return prevents mutable shared arrays.
func curlyCenters(shape int) [6][2]int64 {
	switch shape {
	case 0:
		return [6][2]int64{{73, 69}, {80, 63}, {93, 59}, {107, 59}, {120, 63}, {127, 69}}
	case 1:
		return [6][2]int64{{72, 71}, {79, 65}, {92, 61}, {108, 61}, {121, 65}, {128, 71}}
	case 2:
		return [6][2]int64{{77, 63}, {83, 57}, {94, 53}, {106, 53}, {117, 57}, {123, 63}}
	case 3:
		return [6][2]int64{{69, 71}, {77, 65}, {92, 61}, {108, 61}, {123, 65}, {131, 71}}
	default:
		panic("unvalidated portrait shape")
	}
}

func scalp(d *document, style, shape int, c, t, s int64, hair, skin string) {
	var p path
	switch style {
	case 0:
		x, y := u(c)-35*s, u(t+9)
		d.element("ellipse", n("cx", x), n("cy", y), whole("rx", 10), whole("ry", 5), a("fill", "#ffffff"), a("opacity", "0.1"), a("transform", "rotate(-18 "+number(x)+" "+number(y)+")"))
	case 1:
		d.path(head(c, t-1, s, 84, s-4), hair, a("opacity", "0.92"))
	case 2:
		p.cmd("M", u(c-s), u(92))
		p.cmd("C", u(c-s), u(t+2), u(c)-50*s, u(t-3), u(c), u(t-3))
		p.cmd("C", u(c)+50*s, u(t-3), u(c+s), u(t+2), u(c+s), u(92))
		p.cmd("L", u(c+s), u(80))
		p.cmd("L", u(c-s), u(80))
		p.close()
		d.path(p.String(), hair)
	case 3:
		p.cmd("M", u(c-7), u(90))
		p.cmd("C", u(c-11), u(t-6), u(c-5), u(t-22), u(c), u(t-24))
		p.cmd("C", u(c+5), u(t-22), u(c+11), u(t-6), u(c+7), u(90))
		p.close()
		d.path(p.String(), hair)
	case 4:
		p.cmd("M", u(c-s), u(94))
		p.cmd("C", u(c-s-2), u(t-2), u(c)-40*s, u(t-8), u(c+6), u(t-6))
		p.cmd("C", u(c)+80*s, u(t-4), u(c+s+2), u(t+8), u(c+s), u(88))
		p.cmd("C", u(c)+40*s, u(t+16), u(c)-50*s, u(t+14), u(c-s), u(94))
		p.close()
		d.path(p.String(), hair)
	case 5:
		for _, point := range curlyCenters(shape) {
			d.element("circle", whole("cx", point[0]), whole("cy", point[1]), n("r", 850), a("fill", hair))
		}
		d.path(head(c, t, s-2, 76, s-6), hair)
	case 6:
		p.cmd("M", u(c-s+4), u(84))
		p.cmd("L", u(c-s+2), u(t+2))
		p.cmd("L", u(c+s-2), u(t+2))
		p.cmd("L", u(c+s-4), u(84))
		p.cmd("C", u(c)+40*s, u(t+14), u(c)-40*s, u(t+14), u(c-s+4), u(84))
		p.close()
		d.path(p.String(), hair)
	case 7:
		d.path(head(c, t-1, s, 80, s-3), hair)
		d.path(head(c, t+3, s-3, 84, s-6), skin)
		d.path(head(c, t+3, s-3, 80, s-6), hair, a("opacity", "0.55"))
	default:
		d.err = ErrPortrait
	}
}

func features(d *document, brow, mouth, facial int, scar, hair, shadow string) {
	var brows [2]string
	switch brow {
	case 0:
		brows = [2]string{"M 83,83 L 95,83", "M 105,83 L 117,83"}
	case 1:
		brows = [2]string{"M 83,81 L 95,85", "M 117,81 L 105,85"}
	case 2:
		brows = [2]string{"M 83,84 Q 89,79 95,84", "M 105,84 Q 111,79 117,84"}
	default:
		d.err = ErrPortrait
		return
	}
	for _, data := range brows {
		d.path(data, "none", lineAttrs(hair, "3.4", "", true)...)
	}
	for _, x := range []int64{89, 111} {
		d.element("ellipse", whole("cx", x), whole("cy", 93), n("rx", 540), n("ry", 340), a("fill", "#ece5d8"))
		d.element("circle", whole("cx", x), n("cy", 9340), n("r", 190), a("fill", "#1c1712"))
		d.element("circle", n("cx", u(x)+70), n("cy", 9260), n("r", 60), a("fill", "#ffffff"))
	}
	d.path("M 100,99 L 97,108 Q 100,110.5 103,108", "none", lineAttrs(shadow, "1.8", "", true)...)
	Y := int64(117)
	if facial == 4 {
		Y = 115
	}
	var p path
	switch mouth {
	case 0:
		p.cmd("M", u(92), u(Y))
		p.cmd("L", u(108), u(Y))
		d.path(p.String(), "none", lineAttrs("#6e4630", "2.4", "", true)...)
	case 1:
		p.cmd("M", u(92), u(Y+1))
		p.cmd("Q", u(102), u(Y+2), u(109), u(Y-2))
		d.path(p.String(), "none", lineAttrs("#6e4630", "2.4", "", true)...)
	case 2:
		d.element("rect", whole("x", 91), whole("y", Y-3), whole("width", 18), whole("height", 7), whole("rx", 3), a("fill", "#4a2c1c"))
		d.element("rect", whole("x", 91), whole("y", Y-3), whole("width", 18), whole("height", 3), n("rx", 150), a("fill", "#e8e2d4"))
	case 3:
		d.element("rect", whole("x", 90), whole("y", Y-3), whole("width", 20), n("height", 650), whole("rx", 2), a("fill", "#e8e2d4"), a("stroke", "#6e4630"), a("stroke-width", "1.2"))
		for _, x := range []int64{9650, 10350} {
			var tooth path
			tooth.cmd("M", x, u(Y-3))
			tooth.cmd("L", x, u(Y)+350)
			d.path(tooth.String(), "none", lineAttrs("#6e4630", "1", "", false)...)
		}
	default:
		d.err = ErrPortrait
		return
	}
	switch scar {
	case "none":
	case "left_brow":
		d.path("M 82,79 L 93,86", "none", lineAttrs("#e8c9a0", "1.7", "0.85", true)...)
	case "right_cheek":
		d.path("M 113,103 L 118,111", "none", lineAttrs("#e8c9a0", "1.7", "0.85", true)...)
	default:
		d.err = ErrPortrait
	}
}
