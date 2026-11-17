#!/usr/bin/env python3
"""Independent set39 transport corpus. No SLUG Go package or output is read.

The cryptographic job/proof/receipt values are copied without alteration from
the frozen, independently checked set36/37 public corpus. This script defines
only the new transport envelopes, trust preimages, schema mutations and their
explicit outcomes. It is not an alternative production admission verifier.
"""
import argparse
import base64
import copy
import hashlib
import ipaddress
import json
from pathlib import Path
import re
import ssl
import struct
import tempfile
from urllib.parse import urlsplit

HERE = Path(__file__).resolve().parent
INPUTS = HERE / "inputs"
OUT = HERE / "manifest_service_v1.json"
CONTRACT_SHA = "c25e2fb7f7361a0207acea952c44d62e7fcc9a7ebeeba5887fb2cf3d2b9ebfdf"
ADMISSION_SHA = "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794"
MAX_CONFIG = 65536
MAX_RESPONSE = 73728
DOMAIN = b"SLUG manifest transport trust\x00v1\x00"
ASCII_WS = " \t\r\n"
P = 2**256 - 2**32 - 977


def canonical(value):
    return json.dumps(value, ensure_ascii=True, separators=(",", ":"))


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message="invalid fixture configuration"):
    if not condition:
        raise ValueError(message)


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result, "duplicate key")
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=pairs,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError("constant")))


def fields(value, names):
    require(type(value) is dict and set(value) == set(names))
    require(all(value[n] is not None for n in names))


def integer(value, low, high):
    require(type(value) is int and low <= value <= high)


def hexvalue(value, length, lowercase=False):
    require(type(value) is str and len(value) == 2 * length)
    require(re.fullmatch(r"[0-9a-f]+" if lowercase else r"[0-9a-fA-F]+", value) is not None)
    return value.lower()


def public_key(value):
    value = hexvalue(value, 33)
    require(value[:2] in ("02", "03"))
    x = int(value[2:], 16)
    require(x < P and pow((x*x*x+7) % P, (P-1)//2, P) == 1)
    return value


def public_multiple(number):
    # Public deterministic test labels 1..65, never generated wallet material.
    point = None
    addend = (int("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",16),
              int("483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8",16))
    def add(a,b):
        if a is None:return b
        if b is None:return a
        x,y=a;u,v=b
        if x==u and (y+v)%P==0:return None
        slope=((3*x*x)*pow(2*y,P-2,P) if a==b else (v-y)*pow(u-x,P-2,P))%P
        out=(slope*slope-x-u)%P
        return out,(slope*(x-out)-y)%P
    while number:
        if number&1:point=add(point,addend)
        addend=add(addend,addend);number>>=1
    return ("03" if point[1]&1 else "02")+format(point[0],"064x")


def pin_value(value, known):
    fields(value, ("version", "network", "genesis", "authority_id", "descriptor_id"))
    for key in ("version", "network"):
        require(type(value[key]) is int and value[key] == known[key])
    for key in ("genesis", "authority_id", "descriptor_id"):
        hexvalue(value[key], 32, True)
    require(value["genesis"] == known["genesis"] and value["descriptor_id"] == known["descriptor_id"])
    return copy.deepcopy(value)


def origin_value(value, ca):
    # The selected fixture cases exercise the exact inherited net/url grammar.
    # HTTPS host/port spelling remains distinct, as the approved contract says.
    require(type(value) is str and 0 < len(value.encode()) <= MAX_CONFIG)
    require(not any(ord(c) <= 32 or ord(c) == 127 for c in value))
    require("#" not in value and "?" not in value and "\\" not in value)
    u = urlsplit(value)
    require(u.scheme in ("http", "https") and u.netloc and u.hostname)
    require("@" not in u.netloc and "%" not in u.netloc)
    require(u.path in ("", "/") and not u.query and not u.fragment)
    host_port = u.netloc
    if host_port.startswith("["):
        close = host_port.find("]")
        require(close > 1)
        host = host_port[1:close]
        suffix = host_port[close+1:]
        require(suffix == "" or suffix.startswith(":"))
        port = suffix[1:] if suffix else None
        ipaddress.IPv6Address(host)
    else:
        require(host_port.count(":") <= 1)
        host, separator, port = host_port.partition(":")
        port = port if separator else None
        require(host != "")
    if port is not None:
        require(re.fullmatch(r"[1-9][0-9]*", port) is not None and int(port) <= 65535)
    if u.scheme == "http":
        ip = ipaddress.ip_address(host)
        # Match Go's IPv4-mapped loopback classification too.
        if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped:
            ip = ip.ipv4_mapped
        require(ip.is_loopback and ca == "")
        if port is None:
            host_port += ":80"
    return u.scheme + "://" + host_port


DER_CHECKED = set()


def pem_set(value):
    require(type(value) is str and len(value.encode()) <= MAX_CONFIG)
    if value == "":
        return "", []
    remaining = value
    certs = {}
    while remaining:
        remaining = remaining.lstrip(ASCII_WS)
        if not remaining:
            break
        require(remaining.startswith("-----BEGIN CERTIFICATE-----"))
        end = remaining.find("-----END CERTIFICATE-----")
        require(end >= 0)
        body = remaining[len("-----BEGIN CERTIFICATE-----"):end]
        require(body.startswith("\n") or body.startswith("\r\n"))
        require(re.fullmatch(r"[A-Za-z0-9+/= \t\r\n]+", body) is not None)
        der = base64.b64decode("".join(body.split()), validate=True)
        require(der != b"")
        fingerprint = hashlib.sha256(der).digest()
        require(fingerprint not in certs or certs[fingerprint][0] == der)
        encoded = base64.b64encode(der).decode()
        pem = "-----BEGIN CERTIFICATE-----\n" + "\n".join(encoded[i:i+64] for i in range(0, len(encoded), 64)) + "\n-----END CERTIFICATE-----\n"
        if der not in DER_CHECKED:
            with tempfile.TemporaryDirectory(prefix="slug-public-cert-") as directory:
                path = Path(directory) / "certificate.pem"
                path.write_text(pem)
                ssl._ssl._test_decode_cert(str(path))
            DER_CHECKED.add(der)
        certs[fingerprint] = (der, pem)
        remaining = remaining[end + len("-----END CERTIFICATE-----"):]
    require(certs)
    return "".join(certs[k][1] for k in sorted(certs)), sorted(certs)


def client_config(value, known):
    fields(value, ("version", "url", "token", "ca_certificate", "authority_pin"))
    require(type(value["version"]) is int and value["version"] == 3)
    token = hexvalue(value["token"], 32)
    ca, fingerprints = pem_set(value["ca_certificate"])
    origin = origin_value(value["url"], ca)
    pin = pin_value(value["authority_pin"], known)
    result = dict(version=3, url=origin, token=token, ca_certificate=ca, authority_pin=pin)
    require(len(canonical(result).encode()) <= MAX_CONFIG)
    mode = 1 if origin.startswith("http:") else (3 if fingerprints else 2)
    encoded = origin.encode()
    preimage = DOMAIN + struct.pack("<I", len(encoded)) + encoded + bytes([mode]) + struct.pack("<I", len(fingerprints)) + b"".join(fingerprints)
    return result, dict(mode=mode, certificate_fingerprints=[f.hex() for f in fingerprints],
                        preimage_hex=preimage.hex(), transport_trust_id=sha(preimage))


def server_config(value, known):
    fields(value, ("version", "listen", "authority_pin", "clients", "max_jobs", "max_records"))
    require(type(value["version"]) is int and value["version"] == 3)
    listen = value["listen"]
    require(type(listen) is str and len(listen) <= MAX_CONFIG)
    if listen.startswith("["):
        close = listen.find("]")
        require(close > 0 and listen[close+1:close+2] == ":")
        host, port = listen[1:close], listen[close+2:]
    else:
        require(listen.count(":") == 1)
        host, port = listen.split(":")
    ip = ipaddress.ip_address(host)
    if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped:
        ip = ip.ipv4_mapped
    require(ip.is_loopback and re.fullmatch(r"0|[1-9][0-9]*", port) is not None and int(port) <= 65535)
    listen = ("["+str(ip)+"]" if ip.version == 6 else str(ip)) + ":" + port
    clients = value["clients"]
    require(type(clients) is list and len(clients) <= 64)
    keys, hashes, normalized = set(), set(), []
    for c in clients:
        fields(c, ("public_key", "token_sha256"))
        key, digest = public_key(c["public_key"]), hexvalue(c["token_sha256"], 32)
        require(key not in keys and digest not in hashes)
        keys.add(key); hashes.add(digest)
        normalized.append(dict(public_key=key, token_sha256=digest))
    for k in ("max_jobs", "max_records"):
        integer(value[k], 1, 1000000000)
    result = dict(version=3, listen=listen, authority_pin=pin_value(value["authority_pin"], known),
                  clients=normalized, max_jobs=value["max_jobs"], max_records=value["max_records"])
    require(len(canonical(result).encode()) <= MAX_CONFIG)
    return result


def verify_inputs():
    pins = json.loads((HERE / "input_hashes.json").read_text())
    for name, expected in pins.items():
        require(sha((HERE / name).read_bytes()) == expected, "changed original input: " + name)
    require(sha((INPUTS / "set39-manifest-service-worker-contract.md").read_bytes()) == CONTRACT_SHA)
    require(sha((INPUTS / "admission_reference/manifest_admission_v1.json").read_bytes()) == ADMISSION_SHA)
    return pins


def mutate(obj, path, value):
    result = copy.deepcopy(obj)
    parent = result
    for key in path[:-1]:
        parent = parent[key]
    parent[path[-1]] = value
    return result


def admission(record, inserted):
    return dict(version=3, context_source="trusted-node-snapshot" if inserted else "historical-receipt",
                current_node_checked=inserted, authentication="gym-key-control", birth_rule="job-issuance",
                inserted=inserted, record=copy.deepcopy(record))


def policy(context):
    return {k: context[k] for k in ("version", "network", "genesis", "dna_version", "prospect_target")}


def object_paths(obj, prefix=()):
    yield prefix, obj
    for key, value in obj.items():
        if type(value) is dict:
            yield from object_paths(value, prefix+(key,))


def replace_raw_object(obj, path, transform):
    # Preserve raw duplicate-key syntax without a lossy JSON parse/reencode.
    if not path:
        return transform(canonical(obj))
    return "{" + ",".join(canonical(k)+":"+(replace_raw_object(v, path[1:], transform) if k == path[0] else canonical(v)) for k, v in obj.items()) + "}"


def generate():
    pins = verify_inputs()
    old = json.loads((INPUTS / "admission_reference/manifest_admission_v1.json").read_text())
    jobs_ref = json.loads((INPUTS / "admission_reference/inputs/job_reference/manifest_job_v1.json").read_text())
    known = old["pin"]
    fixtures = old["cases"]
    token = "ab" * 32
    ca_a = (INPUTS / "go_localhost.pem").read_text()
    ca_b = (INPUTS / "go_ecdsa_p256.pem").read_text()
    configs = []
    base = dict(version=3, url="http://127.0.0.1:29334", token=token,
                ca_certificate="", authority_pin=known)

    def config_case(name, kind, value=None, raw=None, accepted=True):
        raw = canonical(value) if raw is None else raw
        expected = None
        try:
            require(len(raw.encode()) <= MAX_CONFIG)
            parsed = strict_json(raw)
            if kind == "client":
                normalized, trust = client_config(parsed, known)
                expected = dict(config=normalized, canonical=canonical(normalized), **trust)
            else:
                normalized = server_config(parsed, known)
                expected = dict(config=normalized, canonical=canonical(normalized),
                                static_constructor_allowed=len(normalized["clients"]) > 0,
                                session_constructor_allowed=len(normalized["clients"]) == 0)
            actual = True
        except (ValueError, TypeError, OverflowError, ssl.SSLError):
            actual = False
        require(actual == accepted, "independent config expectation mismatch: " + name)
        configs.append(dict(name=name, kind=kind, raw=raw, accepted=accepted, expected=expected))

    for name, url in [("http-explicit",base["url"]),("http-implicit","http://127.0.0.1"),
                      ("http-root-slash","http://127.0.0.1/"),("http-ipv6","http://[::1]"),
                      ("http-other-loopback","http://127.2.3.4:5"),("https-system","https://example.invalid"),
                      ("https-explicit443","https://example.invalid:443"),("https-preserve-case","https://EXAMPLE.invalid/"),
                      ("https-ipv6-spelling","https://[0:0:0:0:0:0:0:1]:443")]:
        config_case(name,"client",dict(base,url=url))
    config_case("token-uppercase-normalized","client",dict(base,token=token.upper()))
    config_case("token-rotation-excluded-from-trust","client",dict(base,token="cd"*32))
    foreign_pin = dict(known,authority_id="11"*32)
    config_case("different-explicit-authority-excluded-from-trust","client",dict(base,authority_pin=foreign_pin))
    for name, ca in [("one",ca_a),("other",ca_b),("two-ab",ca_a+ca_b),("two-ba",ca_b+ca_a),
                     ("duplicates",ca_a+ca_b+ca_a),("whitespace"," \t\r\n"+ca_b+"\t \n"+ca_a+"\r\n"),
                     ("crlf",(ca_a+ca_b).replace("\n","\r\n"))]:
        config_case("https-custom-"+name,"client",dict(base,url="https://example.invalid",ca_certificate=ca))
    for key, value in [("version",2),("version",4),("version",3.0),("token","ab"),("token","g"*64),
                       ("url","http://example.invalid"),("url","http://192.0.2.1"),("url","http://0.0.0.0"),
                       ("url","https://example.invalid:"),("url","https://example.invalid:0"),
                       ("url","https://example.invalid:0443"),("url","https://example.invalid:65536"),
                       ("url","https://user@example.invalid"),("url","https://example.invalid/path"),
                       ("url","https://example.invalid/?"),("url","https://example.invalid#"),
                       ("url","https://example.invalid/%2f"),("url","http://[::1%25lo0]"),
                       ("url","ftp://example.invalid"),("url","https://example.invalid\n")]:
        config_case("reject-client-"+key+"-"+str(len(configs)),"client",dict(base,**{key:value}),accepted=False)
    for key in base:
        missing=dict(base);del missing[key]
        config_case("client-missing-"+key,"client",missing,accepted=False)
        config_case("client-null-"+key,"client",dict(base,**{key:None}),accepted=False)
    for key in known:
        bad=copy.deepcopy(base);bad["authority_pin"]=dict(known,**{key:None})
        config_case("client-pin-null-"+key,"client",bad,accepted=False)
    for name, ca in [("http-ca",ca_a),("garbage-prefix","junk\n"+ca_a),("garbage-between",ca_a+"junk\n"+ca_b),
                     ("garbage-suffix",ca_a+"junk"),("only-whitespace"," \t\r\n"),
                     ("unicode-space","\u00a0"+ca_a),("form-feed","\f"+ca_a),
                     ("wrong-label",ca_a.replace("CERTIFICATE","PUBLIC KEY")),
                     ("pem-header",ca_a.replace("-----BEGIN CERTIFICATE-----\n","-----BEGIN CERTIFICATE-----\nComment: test\n\n")),
                     ("bad-der","-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----\n"),
                     ("missing-end",ca_a.split("-----END")[0])]:
        config_case("reject-ca-"+name,"client",dict(base,url=base["url"] if name=="http-ca" else "https://example.invalid",ca_certificate=ca),accepted=False)
    config_case("client-escaped-key-order-whitespace","client",raw=' { "authority_pin":'+canonical(known)+', "ca_certificate":"", "token":'+canonical(token)+', "url":'+canonical(base["url"])+', "v\\u0065rsion":3 } ')
    config_case("client-duplicate-decoded-key","client",raw=canonical(base)[:-1]+',"v\\u0065rsion":3}',accepted=False)
    config_case("client-case-alias","client",dict(base,Version=3),accepted=False)
    config_case("client-trailing-value","client",raw=canonical(base)+'{}',accepted=False)
    config_case("client-exact-raw-limit","client",raw=canonical(base)+' '*(MAX_CONFIG-len(canonical(base))))
    config_case("client-over-raw-limit","client",raw=canonical(base)+' '*(MAX_CONFIG+1-len(canonical(base))),accepted=False)
    # A compact PEM body grows when normalized to canonical64-column PEM.
    # The raw input fits, but its successful typed output would not round-trip.
    one_line_ca="-----BEGIN CERTIFICATE-----\n"+"".join(ca_a.splitlines()[1:-1])+"\n-----END CERTIFICATE-----"
    expanding=dict(base,url="https://x.invalid",ca_certificate=one_line_ca)
    expanding["url"]="https://"+"x"*(1+MAX_CONFIG-len(canonical(expanding)))+".invalid"
    require(len(canonical(expanding))==MAX_CONFIG)
    config_case("client-canonical-normalization-over-limit","client",expanding,accepted=False)
    credentials=[dict(public_key=fixtures[i]["job"]["public_key"],token_sha256=sha(bytes.fromhex(token))) for i in (0,3)]
    credentials[1]["token_sha256"]=sha(bytes.fromhex("cd"*32))
    server=dict(version=3,listen="127.0.0.1:0",authority_pin=known,clients=credentials,max_jobs=10,max_records=10)
    config_case("server-static-two-fixtures","server",server)
    config_case("server-explicit-session-empty-clients","server",dict(server,clients=[]))
    config_case("server-ipv6-canonicalization","server",dict(server,listen="[0:0:0:0:0:0:0:1]:29334"))
    config_case("server-key-token-case-normalized","server",dict(server,clients=[{k:v.upper() for k,v in credentials[0].items()}]))
    many = [dict(public_key=public_multiple(i),token_sha256=sha(("published-credential-label-"+str(i)).encode())) for i in range(1,66)]
    require(many[0]["public_key"]==fixtures[0]["job"]["public_key"] and many[1]["public_key"]==fixtures[3]["job"]["public_key"])
    config_case("server-exact64-distinct-public-clients","server",dict(server,clients=many[:64]))
    config_case("server-over64-distinct-public-clients","server",dict(server,clients=many),accepted=False)
    config_case("server-max-capacities","server",dict(server,max_jobs=1000000000,max_records=1000000000))
    for key,value in [("version",2),("version",3.0),("listen","localhost:29334"),("listen","0.0.0.0:0"),
                      ("listen","127.0.0.1:00"),("listen","127.0.0.1:65536"),("clients",None),
                      ("clients",credentials[:1]*2),("clients",[credentials[0],dict(credentials[1],token_sha256=credentials[0]["token_sha256"])]),
                      ("max_jobs",0),("max_jobs",1000000001),("max_records",-1),("max_records",True)]:
        config_case("reject-server-"+key+"-"+str(len(configs)),"server",dict(server,**{key:value}),accepted=False)
    for key in server:
        missing=dict(server);del missing[key]
        config_case("server-missing-"+key,"server",missing,accepted=False)
    config_case("server-extra-field","server",dict(server,authentication="session"),accepted=False)
    config_case("server-duplicate-decoded-key","server",raw=canonical(server)[:-1]+',"clien\\u0074s":[]}',accepted=False)
    config_case("server-exact-raw-limit","server",raw=canonical(server)+' '*(MAX_CONFIG-len(canonical(server))))
    config_case("server-over-raw-limit","server",raw=canonical(server)+' '*(MAX_CONFIG+1-len(canonical(server))),accepted=False)

    # Pin these equivalences directly, independently of any runtime code.
    trust_by_name={c["name"]:c["expected"]["transport_trust_id"] for c in configs if c["accepted"] and c["kind"]=="client"}
    require(trust_by_name["http-implicit"]==trust_by_name["http-root-slash"])
    require(trust_by_name["http-explicit"]==trust_by_name["token-uppercase-normalized"]==trust_by_name["token-rotation-excluded-from-trust"]==trust_by_name["different-explicit-authority-excluded-from-trust"])
    require(len({trust_by_name["https-custom-"+n] for n in ("two-ab","two-ba","duplicates","whitespace","crlf")})==1)
    require(len({trust_by_name[n] for n in ("https-system","https-explicit443","https-preserve-case","https-custom-one","https-custom-other")})==5)

    job_cases=[]
    base_job=jobs_ref["jobs"][0]["job"]
    for case in jobs_ref["job_inputs"]:
        expected=case["expected"]
        accepted=case["accepted"] and all(expected[k]==known[k] for k in ("authority_id","descriptor_id")) and expected["public_key"]==base_job["public_key"] and policy(expected["context"])==policy(base_job["context"])
        job_cases.append(dict(name="frozen36-"+case["name"],body_hex=case["input_hex"],pin=known,
                              requested_public_key=base_job["public_key"],policy=policy(base_job["context"]),
                              accepted=accepted,expected=expected if accepted else None,
                              error_code=None if accepted else "outcome_unknown",outcome_unknown=not accepted))
    for name,extra in [("wrong-explicit-authority-pin",dict(pin=foreign_pin)),
                       ("different-requested-gym",dict(requested_public_key=fixtures[3]["job"]["public_key"])),
                       ("current-target-mismatch",dict(policy=dict(policy(base_job["context"]),prospect_target="7"+"f"*63)))]:
        case=dict(name=name,body_hex=canonical(base_job).encode().hex(),pin=known,
                  requested_public_key=base_job["public_key"],policy=policy(base_job["context"]),
                  accepted=False,expected=None,error_code="outcome_unknown",outcome_unknown=True)
        case.update(extra);job_cases.append(case)

    receipts=[]
    def receipt_case(name, record=None, wrapper=None, raw=None, accepted=False, fixture=0, inserted=False, current_policy=None, submitted_auth=None, reason="exact receipt or transport binding"):
        original=fixtures[fixture]
        if wrapper is None:
            wrapper=admission(original["expected_record"] if record is None else record,inserted)
        body=canonical(wrapper) if raw is None else raw
        receipts.append(dict(name=name,fixture=original["name"],job=original["job"],proof=original["proof"],
                             authorization=submitted_auth or original["authorization"],policy=current_policy or policy(original["job"]["context"]),
                             body=body,accepted=accepted,expected=wrapper if accepted else None,
                             error_code=None if accepted else "outcome_unknown",outcome_unknown=not accepted,reason=reason))
    for i,c in enumerate(fixtures):
        receipt_case(c["name"]+"-new",fixture=i,inserted=True,accepted=True)
        receipt_case(c["name"]+"-historical",fixture=i,accepted=True)
    b=admission(fixtures[0]["expected_record"],False)
    changed_policy=dict(policy(fixtures[0]["job"]["context"]),prospect_target="7"+"f"*63)
    receipt_case("historical-original-job-under-new-target",accepted=True,current_policy=changed_policy)
    receipt_case("new-result-cannot-claim-current-new-target",inserted=True,current_policy=changed_policy)
    for c in old["alternate_authorizations"]:
        i=next(i for i,f in enumerate(fixtures) if f["proof"]==c["proof"])
        receipt_case(c["name"]+"-retain-first-authority-signature",fixture=i,accepted=True,submitted_auth=c["authorization"])
    for c in old["record_mutations"]:
        accepts=c["name"] in ("sequence_above_meta","accepted_above_clock")
        receipt_case("record-"+c["name"],record=c["record"],accepted=accepts,
                     reason="No remote metadata/count/clock witness; positive sequence and in-interval acceptance are transport trust boundaries." if accepts else "Frozen37 semantic mutation; bound to exact submitted job/proof.")
    receipt_case("coherent-accepted-time-is-not-independent-provenance",record=old["trust_limit_case"]["record"],accepted=True)
    receipt_case("maximum-sequence-is-not-local-registry-count",record=dict(fixtures[0]["expected_record"],sequence=2**64-1),accepted=True)
    old_shape=copy.deepcopy(fixtures[0]["expected_record"])
    old_shape["version"]=4
    del old_shape["static_binding"]
    receipt_case("old-schema4-record-is-not-upgraded",record=old_shape)
    for key,value in [("version",2),("version",4),("context_source","caller"),("current_node_checked",True),
                      ("authentication","signed-PoW"),("birth_rule","acceptance-time"),("inserted",True)]:
        receipt_case("wrapper-substitution-"+key,wrapper=dict(b,**{key:value}))
    receipt_case("new-label-on-historical",wrapper=dict(b,context_source="trusted-node-snapshot"))
    for path,obj in object_paths(b):
        label="root" if not path else ".".join(path)
        for key in obj:
            missing=copy.deepcopy(b);target=missing
            for p in path:target=target[p]
            del target[key]
            receipt_case("missing-"+label+"."+key,wrapper=missing)
            receipt_case("null-"+label+"."+key,wrapper=mutate(b,path+(key,),None))
        receipt_case("unknown-"+label,wrapper=mutate(b,path+("unexpected",),1))
        first=next(iter(obj))
        receipt_case("duplicate-"+label,raw=replace_raw_object(b,path,lambda raw:raw[:-1]+','+canonical(first)+':'+canonical(obj[first])+'}'))
        alias=first[:1].upper()+first[1:]
        receipt_case("case-alias-"+label,wrapper=mutate(b,path+(alias,),obj[first]))
    receipt_case("whitespace-and-reordered-fields",raw=json.dumps(b,sort_keys=True,indent=2),accepted=True)
    receipt_case("escaped-decoded-keys",raw=canonical(b).replace('"version"','"v\\u0065rsion"'),accepted=True)
    receipt_case("escaped-decoded-hex-value",raw=canonical(b).replace('"47528e','"\\u00347528e',1),accepted=True)
    receipt_case("decoded-duplicate-escaped-root-key",raw=canonical(b)[:-1]+',"v\\u0065rsion":3}')
    for path,obj in object_paths(b):
        for key,value in obj.items():
            if type(value) is int:
                raw=replace_raw_object(b,path,lambda raw,k=key,v=value:raw.replace(canonical(k)+':'+str(v),canonical(k)+':'+str(v)+'.0',1))
                receipt_case("noninteger-number-"+".".join(path+(key,)),raw=raw)
    for key,value in [("merkle_branch",None),("merkle_branch",["00"*32]*21),("merkle_branch",{}),
                      ("header","00"*81),("coinbase","00"*16385),("gym_id","0X"+"00"*32)]:
        receipt_case("proof-bound-"+key+"-"+str(len(receipts)),wrapper=mutate(b,("record","proof",key),value))
    receipt_case("depth-six-array",wrapper=mutate(b,("record","verified","dna","attributes","power"),[38]))
    receipt_case("depth-six-object",wrapper=mutate(b,("record","verified","dna","attributes","power"),{"value":38}))
    receipt_case("trailing-value",raw=canonical(b)+'{}')
    receipt_case("invalid-utf8-as-surrogate",raw=canonical(b).replace('"historical-receipt"','"\\ud800"'))
    # Exercise raw caps independently: whitespace is transport-tolerated but
    # each nested public component must remain within its own exact byte cap.
    for path,limit in [(('record',),65536),(('record','job'),8192),(('record','context'),4096),
                       (('record','proof'),49152),(('record','authorization'),4096)]:
        target=b
        for k in path:target=target[k]
        def padded(raw,limit=limit):return raw[:-1]+" "*(limit-len(raw))+"}"
        raw=replace_raw_object(b,path,padded)
        receipt_case("nested-exact-"+".".join(path),raw=raw,accepted=True)
        raw=replace_raw_object(b,path,lambda x,padded=padded:padded(x)[:-1]+" }")
        receipt_case("nested-over-"+".".join(path),raw=raw)
    receipt_case("response-exact-limit",raw=canonical(b)+' '*(MAX_RESPONSE-len(canonical(b))),accepted=True)
    receipt_case("response-over-limit",raw=canonical(b)+' '*(MAX_RESPONSE+1-len(canonical(b))))

    errors=[]
    statuses={"unauthorized":401,"forbidden":403,"invalid_request":400,"too_large":413,"rate_limited":429,
              "expired":409,"unknown_job":404,"rejected":422,"busy":503,"capacity":503,"unavailable":503,"outcome_unknown":503}
    for code,status in statuses.items():
        body=dict(version=3,error=dict(code=code,outcome_unknown=code=="outcome_unknown"))
        errors.append(dict(name=code,status=status,body=canonical(body),wire_valid=True,expected_code=code,expected_unknown=code=="outcome_unknown"))
        errors.append(dict(name=code+"-wrong-flag",status=status,body=canonical(mutate(body,("error","outcome_unknown"),code!="outcome_unknown")),wire_valid=False,expected_code="outcome_unknown",expected_unknown=True))
        errors.append(dict(name=code+"-wrong-status",status=418,body=canonical(body),wire_valid=False,expected_code="outcome_unknown",expected_unknown=True))
    for name,body in [("old-version",dict(version=2,error=dict(code="expired",outcome_unknown=False))),
                      ("unknown-code",dict(version=3,error=dict(code="server_detail",outcome_unknown=False))),
                      ("extra-error-member",dict(version=3,error=dict(code="expired",outcome_unknown=False,detail="fixture-private-detail")))]:
        errors.append(dict(name=name,status=409,body=canonical(body),wire_valid=False,expected_code="outcome_unknown",expected_unknown=True))
    result=dict(oracle_version=1,approved_contract_sha256=CONTRACT_SHA,candidate_fixture_only=True,
                provenance=pins,no_actual_issuance_or_admission=True,
                pin=known,source_fixtures=[dict(name=f["name"],job=f["job"],proof=f["proof"],authorization=f["authorization"],record=f["expected_record"]) for f in fixtures],
                config_cases=configs,job_cases=job_cases,receipt_cases=receipts,error_cases=errors,
                worker_config=dict(version=2,policy=policy(fixtures[0]["job"]["context"]),public_key=fixtures[0]["job"]["public_key"],payout_address=fixtures[0]["candidate"]["payout_address"],authority_origin=base["url"],authority_pin=known,transport_trust_id=trust_by_name["http-explicit"],max_records=1000,max_bytes=67108864),
                trust_boundary="A valid client response remains a statement of the configured authenticated authority; sequence/accepted-time and historical node observations are not independently authenticated by this public JSON.",
                validation=dict(config_cases=len(configs),config_accepted=sum(c["accepted"] for c in configs),job_cases=len(job_cases),job_accepted=sum(c["accepted"] for c in job_cases),receipt_cases=len(receipts),receipt_accepted=sum(c["accepted"] for c in receipts),error_cases=len(errors),max_valid_receipt_body=max(len(c["body"].encode()) for c in receipts if c["accepted"])))
    return (json.dumps(result,indent=2,ensure_ascii=True)+"\n").encode()


def main():
    parser=argparse.ArgumentParser();parser.add_argument("--check",action="store_true");args=parser.parse_args()
    raw=generate()
    if args.check:
        require(OUT.read_bytes()==raw,"generated corpus differs")
    else:
        OUT.write_bytes(raw)
    print(json.dumps(dict(status="passed",sha256=sha(raw),bytes=len(raw),**json.loads(raw)["validation"]),sort_keys=True))


if __name__ == "__main__":
    main()
