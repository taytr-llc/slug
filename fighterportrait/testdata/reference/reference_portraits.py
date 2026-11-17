#!/usr/bin/env python3
"""Independent, standard-library-only SVG oracle for the approved preview contract.

Expected bytes come from this rational implementation, never Go output. The
pinned source Python is used only afterward as a numerical geometry cross-check.
No rasterizer, image tool, network, keys or official fighter assignment is used.
"""
import argparse
import copy
from decimal import Decimal, localcontext, ROUND_HALF_EVEN
from fractions import Fraction as Q
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
VERSION = 1
RENDERER = "slug-fighter-portrait-candidate-v1"
DOMAIN = b"SLUG fighter portrait SVG\x00v1\x00"
MAX_SVG = 32768
NS = "http://www.w3.org/2000/svg"
DESC = "Deterministic appearance preview only. No work, ownership or birth verification."
CONTRACT_HASH = "8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5"
# Independently computed literals, not values read from a renderer.
CURLS = (
    ((73,69),(80,63),(93,59),(107,59),(120,63),(127,69)),
    ((72,71),(79,65),(92,61),(108,61),(121,65),(128,71)),
    ((77,63),(83,57),(94,53),(106,53),(117,57),(123,63)),
    ((69,71),(77,65),(92,61),(108,61),(123,65),(131,71)),
)
ORDER = {
    "rect": ("x","y","width","height","rx","fill","stroke","stroke-width","stroke-linecap","opacity"),
    "ellipse": ("cx","cy","rx","ry","fill","stroke","stroke-width","stroke-linecap","opacity","transform"),
    "circle": ("cx","cy","r","fill","stroke","stroke-width","stroke-linecap","opacity"),
    "path": ("d","fill","stroke","stroke-width","stroke-linecap","opacity"),
}
NUM = re.compile(r"-?(?:0|[1-9][0-9]*)(?:\.[0-9]{1,2})?\Z")
NUMBER_TOKEN = re.compile(r"[-+]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?|[A-Za-z]+")


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canon(obj):
    return json.dumps(obj,ensure_ascii=True,separators=(",",":"),allow_nan=False).encode("ascii")


def pretty(obj):
    return (json.dumps(obj,ensure_ascii=True,indent=2,allow_nan=False)+"\n").encode("ascii")


def load_module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    module=importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_inputs():
    provenance=json.loads((HERE/"input_hashes.json").read_bytes())
    for path,item in provenance.items():
        if sha((HERE/path).read_bytes()) != item["sha256"]:
            raise ValueError("pinned input changed: "+path)
    if provenance["inputs/portrait-renderer-contract.md"]["sha256"] != CONTRACT_HASH:
        raise ValueError("unapproved contract hash")
    independent=load_module("portrait_independent_manifest",HERE/"inputs/manifest/reference_manifest.py")
    source=load_module("portrait_original_geometry",HERE/"inputs/portraits.py")
    cases=json.loads((HERE/"inputs/manifest/manifests_v1.json").read_bytes())
    return provenance,independent,source,cases


PROVENANCE,MANIFEST,SOURCE,INPUT = load_inputs()
T = MANIFEST.TABLES["portrait"]


def even(value):
    """Our own sign-symmetric exact rational ties-to-even, never built-in float round."""
    value=Q(value)
    sign=-1 if value<0 else 1
    whole,rem=divmod(abs(value.numerator),value.denominator)
    if 2*rem>value.denominator or (2*rem==value.denominator and whole%2):
        whole+=1
    return sign*whole


def f(value):
    value=Q(value)
    hundred=value*100
    if hundred.denominator != 1:
        raise ValueError("coordinate is not an exact hundredth")
    n=hundred.numerator
    whole,rem=divmod(abs(n),100)
    out=("-" if n<0 else "")+str(whole)
    if rem:
        out+="."+f"{rem:02d}".rstrip("0")
    return out


def escape(text):
    for old,new in (("&","&amp;"),("<","&lt;"),(">","&gt;"),('"',"&quot;"),("'","&apos;")):
        text=text.replace(old,new)
    return text


def path(*commands):
    out=[]
    for command in commands:
        letter,*pairs=command
        if letter not in {"M","L","C","Q","Z"}:
            raise ValueError("path command")
        expected={"M":1,"L":1,"C":3,"Q":2,"Z":0}[letter]
        if len(pairs)!=expected:
            raise ValueError("path arity")
        out.append(letter)
        out.extend(f(x)+","+f(y) for x,y in pairs)
    return " ".join(out)


def head(c,t,s,b,j):
    h=b-t
    return path(("M",(c,t)),
        ("C",(c+Q(7,10)*s,t),(c+s,t+Q(35,100)*h),(c+s,Q(t+b,2))),
        ("C",(c+s,b-Q(28,100)*h),(c+j,b),(c,b)),
        ("C",(c-j,b),(c-s,b-Q(28,100)*h),(c-s,Q(t+b,2))),
        ("C",(c-s,t+Q(35,100)*h),(c-Q(7,10)*s,t),(c,t)),("Z",))


class SVG:
    def __init__(self,label):
        text=escape(label)
        self.parts=[f'<svg xmlns="{NS}" width="200" height="200" viewBox="0 0 200 200" preserveAspectRatio="xMidYMid meet" overflow="hidden" role="img" aria-label="{text}" data-renderer="{RENDERER}">',
                    f'<title>{text}</title><desc>{DESC}</desc>']
        self.length=sum(len(x) for x in self.parts)
        if self.length>=1024:
            raise ValueError("SVG root/text envelope bound")
        self.graphics=0
    def add(self,tag,**attrs):
        attrs={k.replace("_","-"):v for k,v in attrs.items()}
        if tag not in ORDER or not set(attrs)<=set(ORDER[tag]):
            raise ValueError("element/attribute allowlist")
        if tag=="path" and "fill" not in attrs:
            raise ValueError("path missing explicit fill")
        out="<"+tag
        for key in ORDER[tag]:
            if key in attrs:
                value=attrs[key]
                value=f(value) if type(value) in (int,Q) else value
                if not isinstance(value,str):
                    raise ValueError("nonexact scalar")
                out+=" "+key+'="'+escape(value)+'"'
        out+="/>"
        if len(out)>=512 or self.graphics>=43 or self.length+len(out)+6>MAX_SVG:
            raise ValueError("SVG output bound")
        self.parts.append(out);self.length+=len(out);self.graphics+=1
    def p(self,d,**attrs):
        self.add("path",d=d,**attrs)
    def finish(self):
        raw=("".join(self.parts)+"</svg>").encode("ascii")
        if not len(raw)<=MAX_SVG:
            raise ValueError("SVG output bound")
        return raw


def _draw(m):
    """Private geometry helper: public render() always regenerates the manifest first."""
    a=m["portrait"]
    skin,shadow=T["skin_colors"][a["skin"]]
    hair=T["hair_colors"][a["hair_color"]]
    bg=T["background_colors"][a["background"]]
    ring=T["ring_by_rarity"][m["dna"]["rarity"]]
    shape=T["head_shapes"][a["head_shape"]]
    c,t,s,b,j=100,shape["top"],shape["side"],shape["bottom"],shape["jaw"]
    S=T["build_by_class"][m["physical"]["weight_class"]]+even(Q(a["shoulder_jitter"],2))
    CI,CO,PO,PM,NX=(even(Q(S*n,100)) for n in (22,46,40,20,26))
    PD=even(Q(16800+13*S,100))
    J=b-2;Y=115 if a["facial_hair"]==4 else 117
    out=SVG("Portrait preview of "+m["name"]["display"])
    out.add("rect",width=200,height=200,fill=bg)
    out.add("ellipse",cx=100,cy=86,rx=92,ry=82,fill="#ffffff",opacity=Q(5,100))
    out.add("rect",x=88,y=110,width=24,height=30,rx=6,fill=shadow)
    out.p(path(("M",(c-17,132)),("C",(c-36,137),(c-S-10,140),(c-S-13,154)),
        ("C",(c-S-15,168),(c-S-12,184),(c-S-11,200)),("L",(c+S+11,200)),
        ("C",(c+S+12,184),(c+S+15,168),(c+S+13,154)),
        ("C",(c+S+10,140),(c+36,137),(c+17,132)),("Q",(c,142),(c-17,132)),("Z",)),fill=skin)
    for q in (-1,1):
        out.p(path(("M",(c+q*(S-8),154)),("C",(c+q*(S+3),168),(c+q*(S-1),186),(c+q*S,200)),
            ("L",(c+q*(S+11),200)),("C",(c+q*(S+12),184),(c+q*(S+15),168),(c+q*(S+13),154)),
            ("C",(c+q*(S+4),146),(c+q*(S-2),148),(c+q*(S-8),154)),("Z",)),fill=shadow,opacity=Q(12,100))
        out.p(path(("M",(c+q*(S-8),154)),("C",(c+q*(S+3),168),(c+q*(S-1),184),(c+q*(S-2),200))),fill="none",stroke=shadow,stroke_width=Q(22,10),stroke_linecap="round",opacity=Q(55,100))
        out.p(path(("M",(c+q*(S+9),144)),("C",(c+q*(S+2),152),(c+q*(S-3),158),(c+q*(S-7),163))),fill="none",stroke=shadow,stroke_width=2,stroke_linecap="round",opacity=Q(4,10))
    for q in (-1,1):
        out.p(path(("M",(c+q*CI,143)),("L",(c+q*CO,147))),fill="none",stroke=shadow,stroke_width=2,stroke_linecap="round",opacity=Q(35,100))
    out.p(path(("M",(c-PO,167)),("Q",(c-PM,PD),(c-2,168))),fill="none",stroke=shadow,stroke_width=2,stroke_linecap="round",opacity=Q(45,100))
    out.p(path(("M",(c+2,168)),("Q",(c+PM,PD),(c+PO,167))),fill="none",stroke=shadow,stroke_width=2,stroke_linecap="round",opacity=Q(45,100))
    out.p(path(("M",(c,150)),("L",(c,150+PM))),fill="none",stroke=shadow,stroke_width=Q(16,10),opacity=Q(25,100))
    for q in (-1,1):
        out.add("circle",cx=c+q*NX,cy=171,r=Q(17,10),fill=shadow,opacity=Q(5,10))
    for q in (-1,1):
        out.add("ellipse",cx=c+q*s,cy=97,rx=5,ry=8,fill=shadow)
    out.p(head(c,t,s,b,j),fill=skin)
    facial=a["facial_hair"]
    if facial==1:
        out.p(path(("M",(c-j-4,J-26)),("C",(c-j,J),(c+j,J),(c+j+4,J-26)),("C",(c+14,J-16),(c-14,J-16),(c-j-4,J-26)),("Z",)),fill=hair,opacity=Q(3,10))
    elif facial in (2,3):
        out.p(path(("M",(c-13,111)),("C",(c-6,107),(c-2,110),(c,112)),("C",(c+2,110),(c+6,107),(c+13,111)),("C",(c+7,116),(c-7,116),(c-13,111)),("Z",)),fill=hair)
        if facial==3:
            out.add("ellipse",cx=c,cy=124,rx=8,ry=6,fill=hair)
    elif facial==4:
        out.p(path(("M",(c-s+3,94)),("C",(c-s+1,J-8),(c-j,J+3),(c,J+3)),("C",(c+j,J+3),(c+s-1,J-8),(c+s-3,94)),("C",(c+15,110),(c-15,110),(c-s+3,94)),("Z",)),fill=hair)
    hs=a["hair_style"]
    if hs==0:
        x=c-Q(35,100)*s;y=t+9
        out.add("ellipse",cx=x,cy=y,rx=10,ry=5,fill="#ffffff",opacity=Q(1,10),transform=f"rotate(-18 {f(x)} {f(y)})")
    elif hs==1:
        out.p(head(c,t-1,s,84,s-4),fill=hair,opacity=Q(92,100))
    elif hs==2:
        out.p(path(("M",(c-s,92)),("C",(c-s,t+2),(c-Q(s,2),t-3),(c,t-3)),("C",(c+Q(s,2),t-3),(c+s,t+2),(c+s,92)),("L",(c+s,80)),("L",(c-s,80)),("Z",)),fill=hair)
    elif hs==3:
        out.p(path(("M",(c-7,90)),("C",(c-11,t-6),(c-5,t-22),(c,t-24)),("C",(c+5,t-22),(c+11,t-6),(c+7,90)),("Z",)),fill=hair)
    elif hs==4:
        out.p(path(("M",(c-s,94)),("C",(c-s-2,t-2),(c-Q(4,10)*s,t-8),(c+6,t-6)),("C",(c+Q(8,10)*s,t-4),(c+s+2,t+8),(c+s,88)),("C",(c+Q(4,10)*s,t+16),(c-Q(s,2),t+14),(c-s,94)),("Z",)),fill=hair)
    elif hs==5:
        for x,y in CURLS[a["head_shape"]]:
            out.add("circle",cx=x,cy=y,r=Q(85,10),fill=hair)
        out.p(head(c,t,s-2,76,s-6),fill=hair)
    elif hs==6:
        out.p(path(("M",(c-s+4,84)),("L",(c-s+2,t+2)),("L",(c+s-2,t+2)),("L",(c+s-4,84)),("C",(c+Q(4,10)*s,t+14),(c-Q(4,10)*s,t+14),(c-s+4,84)),("Z",)),fill=hair)
    elif hs==7:
        out.p(head(c,t-1,s,80,s-3),fill=hair)
        out.p(head(c,t+3,s-3,84,s-6),fill=skin)
        out.p(head(c,t+3,s-3,80,s-6),fill=hair,opacity=Q(55,100))
    else:
        raise ValueError("hair branch")
    brows=(
        (path(("M",(83,83)),("L",(95,83))),path(("M",(105,83)),("L",(117,83)))),
        (path(("M",(83,81)),("L",(95,85))),path(("M",(117,81)),("L",(105,85)))),
        (path(("M",(83,84)),("Q",(89,79),(95,84))),path(("M",(105,84)),("Q",(111,79),(117,84)))),
    )
    for d in brows[a["brow"]]:
        out.p(d,fill="none",stroke=hair,stroke_width=Q(34,10),stroke_linecap="round")
    for x in (89,111):
        out.add("ellipse",cx=x,cy=93,rx=Q(54,10),ry=Q(34,10),fill="#ece5d8")
        out.add("circle",cx=x,cy=Q(934,10),r=Q(19,10),fill="#1c1712")
        out.add("circle",cx=Q(10*x+7,10),cy=Q(926,10),r=Q(6,10),fill="#ffffff")
    out.p(path(("M",(100,99)),("L",(97,108)),("Q",(100,Q(1105,10)),(103,108))),fill="none",stroke=shadow,stroke_width=Q(18,10),stroke_linecap="round")
    mouth=a["mouth"]
    if mouth in (0,1):
        d=path(("M",(92,Y)),("L",(108,Y))) if mouth==0 else path(("M",(92,Y+1)),("Q",(102,Y+2),(109,Y-2)))
        out.p(d,fill="none",stroke="#6e4630",stroke_width=Q(24,10),stroke_linecap="round")
    elif mouth==2:
        out.add("rect",x=91,y=Y-3,width=18,height=7,rx=3,fill="#4a2c1c")
        out.add("rect",x=91,y=Y-3,width=18,height=3,rx=Q(15,10),fill="#e8e2d4")
    else:
        out.add("rect",x=90,y=Y-3,width=20,height=Q(65,10),rx=2,fill="#e8e2d4",stroke="#6e4630",stroke_width=Q(12,10))
        for x in (Q(965,10),Q(1035,10)):
            out.p(path(("M",(x,Y-3)),("L",(x,Y+Q(35,10)))),fill="none",stroke="#6e4630",stroke_width=1)
    scar=a["scar"]
    if scar!="none":
        points=((82,79),(93,86)) if scar=="left_brow" else ((113,103),(118,111))
        out.p(path(("M",points[0]),("L",points[1])),fill="none",stroke="#e8c9a0",stroke_width=Q(17,10),stroke_linecap="round",opacity=Q(85,100))
    out.add("rect",x=Q(25,10),y=Q(25,10),width=195,height=195,rx=12,fill="none",stroke=ring,stroke_width=3)
    return out.finish()


def render(raw,version=VERSION):
    if type(version) is not int or version!=VERSION:
        raise ValueError("unknown renderer version")
    if not isinstance(raw,bytes) or not 1<=len(raw)<=8192:
        raise ValueError("manifest byte limit")
    owned=memoryview(raw).tobytes()
    manifest=MANIFEST.parse_manifest(owned)
    return _draw(manifest)


def validate_xml(raw,label):
    """Positive XML/tag/attribute/value allowlist; no DTD/entity processing path."""
    if not isinstance(raw,bytes) or not 1<=len(raw)<=MAX_SVG or not raw.isascii():
        raise ValueError("SVG bytes")
    if any(x in raw for x in (b"<!",b"<?",b"\n",b"\r",b"\t")):
        raise ValueError("forbidden XML structure")
    envelope="".join(SVG(label).parts).encode("ascii")
    if not raw.startswith(envelope) or not raw.endswith(b"</svg>"):
        raise ValueError("noncanonical envelope bytes")
    root=ET.fromstring(raw)
    attrs={"width":"200","height":"200","viewBox":"0 0 200 200","preserveAspectRatio":"xMidYMid meet","overflow":"hidden","role":"img","aria-label":label,"data-renderer":RENDERER}
    if root.tag!="{"+NS+"}svg" or root.attrib!=attrs or list(root.attrib)!=list(attrs) or root.text is not None:
        raise ValueError("SVG envelope")
    children=list(root)
    if not 2<=len(children)<=45:
        raise ValueError("element bound")
    for index,(tag,text) in enumerate((("title",label),("desc",DESC))):
        element=children[index]
        if element.tag!="{"+NS+"}"+tag or element.attrib or list(element) or element.text!=text or element.tail is not None:
            raise ValueError("accessible text")
    graphic_bytes=raw[len(envelope):-6]
    serialized=re.findall(rb"<[^>]+/>",graphic_bytes)
    if b"".join(serialized)!=graphic_bytes or len(serialized)!=len(children)-2 or any(len(item)>=512 for item in serialized):
        raise ValueError("graphic byte grammar/bound")
    for element in children[2:]:
        tag=element.tag.removeprefix("{"+NS+"}")
        if tag not in ORDER or element.tag!="{"+NS+"}"+tag or list(element) or element.text is not None or element.tail is not None:
            raise ValueError("graphic shape")
        keys=list(element.attrib)
        if keys!=[k for k in ORDER[tag] if k in keys] or not set(keys)<=set(ORDER[tag]) or "fill" not in keys:
            raise ValueError("attribute allowlist/order")
        for key,value in element.attrib.items():
            if key in ("fill","stroke"):
                if not (value=="none" or re.fullmatch(r"#[0-9a-f]{6}",value)):
                    raise ValueError("paint value")
            elif key=="stroke-linecap":
                if value!="round": raise ValueError("linecap")
            elif key=="transform":
                if tag!="ellipse" or not re.fullmatch(r"rotate\(-18 [0-9]+(?:\.[0-9]{1,2})? [0-9]+\)",value):
                    raise ValueError("transform")
            elif key=="d":
                # Rebuild exact command grammar, including pair counts.
                tokens=value.split(" ");pos=0;rebuilt=[]
                while pos<len(tokens):
                    command=tokens[pos];pos+=1
                    if command not in {"M","L","C","Q","Z"}: raise ValueError("path command")
                    pairs=[]
                    for _ in range({"M":1,"L":1,"C":3,"Q":2,"Z":0}[command]):
                        if pos>=len(tokens): raise ValueError("short path")
                        pair=tokens[pos].split(",");pos+=1
                        if len(pair)!=2 or any(not NUM.fullmatch(n) or f(Q(n))!=n for n in pair): raise ValueError("path number")
                        pairs.append(tuple(Q(n) for n in pair))
                    rebuilt.append((command,*pairs))
                if path(*rebuilt)!=value: raise ValueError("path formatting")
            elif not NUM.fullmatch(value) or f(Q(value))!=value:
                raise ValueError("numeric format")
    return {"graphics_count":len(children)-2,"element_count":len(children)+1,"depth":2,"byte_length":len(raw)}


def decimal_trig(x):
    # Taylor evaluated at 90-digit precision; pi below has >90 trusted digits.
    square=x*x
    sin_term=x;cos_term=Decimal(1);sin_total=x;cos_total=Decimal(1)
    for n in range(1,150):
        sin_term *= -square/Decimal((2*n)*(2*n+1))
        cos_term *= -square/Decimal((2*n-1)*(2*n))
        sin_total+=sin_term;cos_total+=cos_term
        if abs(sin_term)<Decimal("1e-85") and abs(cos_term)<Decimal("1e-85"):
            return sin_total,cos_total
    raise AssertionError("trig series bound")


def curly_vectors():
    vectors=[]
    with localcontext() as ctx:
        ctx.prec=90
        pi=Decimal("3.1415926535897932384626433832795028841971693993751058209749445923078164062862089986280348253421170679")
        for index,shape in enumerate(T["head_shapes"]):
            points=[]
            for k in range(6):
                sin,cos=decimal_trig(pi*Decimal(10+21*k)/125)
                x=Decimal(100)-cos*Decimal(shape["side"]-3)
                y=Decimal(shape["top"]+16)-sin*13
                point=(int(x.to_integral_value(rounding=ROUND_HALF_EVEN)),int(y.to_integral_value(rounding=ROUND_HALF_EVEN)))
                assert point==CURLS[index][k]
                points.append(list(point))
            vectors.append({"head_shape":shape["name"],"centers":points,"radius":"8.5"})
    return vectors


def rounding_vectors():
    vectors=[]
    for S in range(51,78):
        exact=[even(Q(S*n,100)) for n in (22,46,40,20)]+[even(Q(16800+13*S,100)),even(Q(26*S,100))]
        prototype=[round(S*n) for n in (.22,.46,.40,.20)]+[round(168+S*.13),round(S*.26)]
        assert exact==prototype
        vectors.append(dict(zip(("shoulder_width","collar_in","collar_out","pec_out","pec_mid","pec_dip","nipple_x"),(S,*exact))))
    for numerator in range(-21,22):
        expected=int((Decimal(numerator)/2).to_integral_value(rounding=ROUND_HALF_EVEN))
        assert even(Q(numerator,2))==expected
    for value,want in [(Q(0),"0"),(Q(-18),"-18"),(Q(25,10),"2.5"),(Q(8355,100),"83.55"),(Q(11056,100),"110.56")]:
        assert f(value)==want
    return vectors


def compare_original(m,expected):
    """Compare independently produced geometry after generating golden bytes.

    The old source trait hash is deliberately replaced with the frozen manifest
    selections. No source function decides any expected oracle geometry/bytes.
    """
    a=m["portrait"]
    traits={"skin":a["skin"],"hair_style":a["hair_style"],"hair_color":a["hair_color"],"facial":a["facial_hair"],"shape":a["head_shape"],"mouth":a["mouth"],"scar":{"none":35,"left_brow":0,"right_cheek":1}[a["scar"]],"brow":a["brow"],"bg":a["background"],"tank":0,"jitter":a["shoulder_jitter"]}
    old=SOURCE._traits
    try:
        SOURCE._traits=lambda _unused:traits.copy()
        original=SOURCE.portrait_svg({"id":"manifest-selected-comparison-only","tier":m["dna"]["rarity"].title(),"wclass":m["physical"]["weight_class"].replace("_"," ").title()})
    finally:
        SOURCE._traits=old
    left=list(ET.fromstring(expected))[2:];right=list(ET.fromstring(original))
    assert len(left)==len(right)
    for index,(a,b) in enumerate(zip(left,right)):
        assert a.tag==b.tag,(index,a.tag,b.tag)
        attrs=b.attrib.copy()
        if a.tag.endswith("}path") and "fill" not in attrs:
            attrs["fill"]="none"
        assert set(a.attrib)==set(attrs),(index,a.attrib,attrs)
        for key,value in a.attrib.items():
            other=attrs[key]
            if key in ("fill","stroke","stroke-linecap"):
                assert value==other,(index,key,value,other)
            else:
                av=NUMBER_TOKEN.findall(value);bv=NUMBER_TOKEN.findall(other)
                assert len(av)==len(bv),(index,key,value,other)
                for x,y in zip(av,bv):
                    if x[0].isalpha(): assert x==y
                    else: assert abs(Decimal(x)-Decimal(y))<Decimal("0.000000000002"),(index,key,x,y)


def negative_checks(raw,svg):
    checks=0
    def reject(fn):
        nonlocal checks
        try: fn()
        except (ValueError,TypeError,KeyError,json.JSONDecodeError,ET.ParseError):
            checks+=1;return
        raise AssertionError("negative input accepted")
    for version in (0,2,-1,True,1.0,"1",None): reject(lambda version=version:render(raw,version))
    for value in (b"",b" "*8193,raw+b"\n",b" "+raw,raw+b"{}",raw.replace(b'"skin":1',b'"skin":true'),raw.replace(b'"skin":1',b'"skin":1.0'),raw.replace(b'"skin":1',b'"skin":1e0'),raw.replace(b'"skin":1',b'"skin":null'),raw.replace(b'"skin":1',b'"skin":1,"skin":1'),raw.replace(b'"skin":1',b'"Skin":1'),raw.replace(b'"skin":1',b'"sk\\u0069n":1')):
        reject(lambda value=value:render(value))
    for fixture in INPUT["invalid_manifest_inputs"]:
        assert fixture["accepted"] is False
        value=bytes.fromhex(fixture["input_hex"])
        reject(lambda value=value:render(value))
    base=json.loads(raw)
    paths=[("manifest_version",),("generator_id",),("table_hash",),("name","first"),("name","last"),("name","display"),("physical","weight_class"),("dna","rarity"),("style",)]
    paths += [("portrait",key) for key in base["portrait"]]
    for keys in paths:
        m=copy.deepcopy(base);parent=m
        for key in keys[:-1]: parent=parent[key]
        value=parent[keys[-1]]
        parent[keys[-1]]=value+1 if type(value) is int else value+"x"
        reject(lambda m=m:render(canon(m)))
    label="Portrait preview of "+base["name"]["display"]
    mutants=[svg.replace(b'<path ',b'<path onclick="x" ',1),svg.replace(b'</svg>',b'<script/></svg>'),svg.replace(b'</svg>',b'<foreignObject/></svg>'),svg.replace(b'</svg>',b'<image href="https://example.invalid/x"/></svg>'),svg.replace(b'fill="#',b'fill="url(#',1),b'<!DOCTYPE svg [<!ENTITY x "bad">]>'+svg,svg.replace(b'width="200"',b'width="200.0"',1),svg.replace(b'<title>',b'<title id="same">',1),svg.replace(b'<svg ',b'<svg style="x" ',1),svg+b'\n',svg.replace(b'</svg>',b'<svg/></svg>')]
    for value in mutants: reject(lambda value=value:validate_xml(value,label))
    assert escape("&<>\"'")=="&amp;&lt;&gt;&quot;&apos;"
    reject(lambda: SVG("x"*1024))
    reject(lambda: f(Q(1,3)))
    limited=SVG(label)
    limited.length=MAX_SVG-6
    reject(lambda:limited.add("circle",cx=0,cy=0,r=1,fill="#ffffff"))
    limited=SVG(label);limited.graphics=43
    reject(lambda:limited.add("circle",cx=0,cy=0,r=1,fill="#ffffff"))
    return checks


def helper_comparisons(base):
    count=0
    # Each possible shoulder width and head/hair combination, plus every branch.
    widths={}
    for class_,build in T["build_by_class"].items():
        for jitter in range(-6,7): widths.setdefault(build+even(Q(jitter,2)),(class_,jitter))
    for S,(class_,jitter) in sorted(widths.items()):
        m=copy.deepcopy(base);m["physical"]["weight_class"]=class_;m["portrait"]["shoulder_jitter"]=jitter
        compare_original(m,_draw(m));count+=1
    for shape in range(4):
        for hair in range(8):
            m=copy.deepcopy(base);m["portrait"].update(head_shape=shape,hair_style=hair)
            compare_original(m,_draw(m));count+=1
    for facial in range(5):
        for mouth in range(4):
            for brow in range(3):
                for scar in ("none","left_brow","right_cheek"):
                    m=copy.deepcopy(base);m["portrait"].update(facial_hair=facial,mouth=mouth,brow=brow,scar=scar)
                    compare_original(m,_draw(m));count+=1
    m=copy.deepcopy(base);m["portrait"].update(hair_style=5,facial_hair=3,mouth=3,scar="left_brow")
    summary=validate_xml(_draw(m),"Portrait preview of "+m["name"]["display"])
    assert summary["graphics_count"]==43 and summary["element_count"]==46
    return count,summary


def corpus():
    records=[];coverage={};graphics_max=0;bytes_max=0
    for case in INPUT["cases"]:
        raw=case["canonical"].encode("ascii")
        m=MANIFEST.parse_manifest(raw)
        assert m==case["manifest"] and MANIFEST.digest_manifest(raw)==case["digest"]
        svg=render(raw)
        assert render(raw)==svg
        stats=validate_xml(svg,"Portrait preview of "+m["name"]["display"])
        compare_original(m,svg)
        records.append({"name":case["name"],"seed":case["seed"],"manifest":m,"manifest_digest":case["digest"],"svg":svg.decode("ascii"),"svg_sha256":sha(svg),"svg_digest":sha(DOMAIN+svg),**stats})
        graphics_max=max(graphics_max,stats["graphics_count"]);bytes_max=max(bytes_max,len(svg))
        for key,value in m["portrait"].items():
            if key!="portrait_version": coverage.setdefault(key,set()).add(value)
        coverage.setdefault("weight_class",set()).add(m["physical"]["weight_class"])
        coverage.setdefault("rarity",set()).add(m["dna"]["rarity"])
    required={"skin":range(6),"hair_style":range(8),"hair_color":range(6),"facial_hair":range(5),"head_shape":range(4),"mouth":range(4),"brow":range(3),"background":range(8),"scar":["none","left_brow","right_cheek"],"shoulder_jitter":range(-6,7),"weight_class":T["build_by_class"],"rarity":T["ring_by_rarity"]}
    for key,values in required.items(): assert coverage[key]==set(values),(key,coverage[key])
    helper_count,maximum=helper_comparisons(records[0]["manifest"])
    negatives=negative_checks(INPUT["cases"][0]["canonical"].encode(),records[0]["svg"].encode())
    return {"preview_only":True,"renderer_version":VERSION,"renderer_id":RENDERER,"digest_domain_hex":DOMAIN.hex(),"approved_contract_sha256":CONTRACT_HASH,"provenance":PROVENANCE,"cases":records,"rounding_vectors":rounding_vectors(),"curly_vectors":curly_vectors(),"jitter_rounding":[{"jitter":i,"rounded_half":even(Q(i,2))} for i in range(-6,7)],"coverage":{k:sorted(v) for k,v in sorted(coverage.items())},"validation":{"manifest_cases":len(records),"original_geometry_comparisons":len(records)+helper_count,"negative_cases":negatives,"maximum_corpus_graphics":graphics_max,"maximum_corpus_bytes":bytes_max,"synthetic_maximum":maximum,"full_pixel_or_accessibility_tests":False}}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    group=parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--generate",action="store_true")
    group.add_argument("--check",action="store_true")
    args=parser.parse_args()
    result=corpus();raw=pretty(result)
    expected=HERE/"portraits_v1.json"
    svgdir=HERE/"svg"
    if args.generate:
        expected.write_bytes(raw);svgdir.mkdir(exist_ok=True)
        for case in result["cases"]: (svgdir/(case["name"]+".svg")).write_bytes(case["svg"].encode("ascii"))
    else:
        if expected.read_bytes()!=raw: raise SystemExit("FAIL: complete corpus differs from independent regeneration")
        if {p.name for p in svgdir.iterdir()}!={x["name"]+".svg" for x in result["cases"]}: raise SystemExit("FAIL: SVG file set")
        for case in result["cases"]:
            if (svgdir/(case["name"]+".svg")).read_bytes()!=case["svg"].encode("ascii"): raise SystemExit("FAIL: SVG bytes "+case["name"])
    print(json.dumps({"result":"pass","mode":"generate" if args.generate else "check","corpus_sha256":sha(raw),"validation":result["validation"]},sort_keys=True))


if __name__=="__main__":
    main()
