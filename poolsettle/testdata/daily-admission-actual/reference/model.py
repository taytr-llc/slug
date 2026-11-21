#!/usr/bin/env python3
"""Frozen-before-execution public Set78 composition and exact economics.

Reuses independent public reference sources. No Go/product imports, node,
real keys, signature generation, mined proof or expected runtime output.
"""
import argparse
import copy
from fractions import Fraction
import hashlib
import json
from pathlib import Path
import runpy
import struct

BASE = Path(__file__).resolve().parent
FILES = (
    "PROPOSAL.md", "recipe.json", "inputs/accounting-policy.md",
    "inputs/capacity-report.json", "inputs/capacity-v1.json", "inputs/crypto.py",
    "inputs/gym-reference.py", "inputs/native84-v1.json", "inputs/set76-actual-v1.json",
    "inputs/set76-model.py", "inputs/setup-v1.json", "inputs/wallet-daily-v1.json",
    "model.py", "PRODUCER.md",
)


def require(condition, label):
    if not condition:
        raise ValueError(label)


def canon(value):
    return json.dumps(value, ensure_ascii=True, allow_nan=False, separators=(",", ":")).encode("ascii")


def digest(value):
    return hashlib.sha256(value).hexdigest()


def read(name):
    return json.loads((BASE / name).read_bytes())


def framed(domain, value):
    raw = canon(value)
    require(len(raw) <= 65536, "bounded identity frame")
    return digest(domain.encode("ascii") + struct.pack("<I", len(raw)) + raw)


def fraction(value):
    value = Fraction(value)
    return {"numerator": str(value.numerator), "denominator": str(value.denominator)}


def economic_projection(reward_a, reward_b, fee, shares, label):
    # No expected amount is copied from the proposed scenario or Go quote.
    net_a = Fraction(99 * reward_a, 100)
    net_b = [Fraction(99 * reward_b * share, 100 * sum(shares)) for share in shares]
    cut_a, cut_b = Fraction(reward_a, 100), Fraction(reward_b, 100)
    require(net_a.denominator == 1 and cut_a.denominator == 1, "integral first reward")
    paid_a = int(net_a)
    change_a = reward_a - paid_a - fee
    require(change_a > 0 and Fraction(change_a) == cut_a - fee, "fee only from operator cut")
    gross = [net_a + net_b[0], net_b[1]]
    outstanding = [gross[0] - paid_a, gross[1]]
    require(outstanding == net_b, "Confirmed excludes previously paid A")
    residual_operator = cut_a + cut_b - fee
    frontier = change_a + reward_b
    require(sum(net_b) + residual_operator == frontier, "exact final frontier conservation")
    require(sum(gross) + cut_a + cut_b == reward_a + reward_b, "lifetime reward conservation")

    def balance(gym, confirmed, available, reserved, paid, reason):
        return {"gym": gym, "confirmed": fraction(confirmed), "available": fraction(available),
                "reserved_atoms": reserved, "paid_atoms": paid, "reason": reason}

    zero = balance("gym2", 0, 0, 0, 0, "")
    pending = [balance("gym1", net_a, 0, paid_a, 0, "broadcast-unknown"), zero]
    recovered = [balance("gym1", 0, 0, 0, paid_a, ""), zero]
    final = [balance("gym1", outstanding[0], 0, 0, paid_a, "daily-batch-complete"),
             balance("gym2", outstanding[1], 0, 0, 0, "daily-batch-complete")]
    return {
        "scope": label, "a_reward_atoms": reward_a, "b_reward_atoms": reward_b,
        "a_fee_atoms": fee, "a_payment_atoms": paid_a, "a_change_atoms": change_a,
        "a_inputs": 1, "a_payments": 1, "a_change_index": 1,
        "round_a_net": fraction(net_a), "round_b_net_by_gym": [fraction(v) for v in net_b],
        "round_b_gross_by_gym": [fraction(Fraction(reward_b * s, sum(shares))) for s in shares],
        "lifetime_net_by_gym": [fraction(v) for v in gross],
        "outstanding_by_gym": [fraction(v) for v in outstanding],
        "operator_a_earned": fraction(cut_a), "operator_b_earned": fraction(cut_b),
        "operator_a_residual": fraction(change_a), "operator_total_residual": fraction(residual_operator),
        "final_frontier_atoms": frontier,
        "final_frontier": [
            {"origin": "A-change", "atoms": change_a, "claims": [{"gym": "operator", "amount": fraction(change_a)}]},
            {"origin": "B-coinbase", "atoms": reward_b,
             "claims": [{"gym": "gym1", "amount": fraction(net_b[0])},
                        {"gym": "gym2", "amount": fraction(net_b[1])},
                        {"gym": "operator", "amount": fraction(cut_b)}]},
        ],
        "reports": {
            "a_lost_reply": {"state": "pending", "reason": "broadcast-unknown", "balances": pending},
            "a_recovered_before_b_credit": {"state": "complete", "reason": "", "balances": recovered},
            "b59_same_day": {"state": "complete", "reason": "daily-batch-complete", "balances": recovered},
            "b60_same_day": {"state": "complete", "reason": "daily-batch-complete", "balances": final},
        },
        "confirmation_semantics": "Confirmed=gross net earnings-Paid, includes Reserved; Available reflects original funding evidence, never lifetime gross.",
    }


def load_lock(path):
    lock = json.loads(path.read_bytes())
    require(lock["version"] == 1 and lock["approved_by"] == "root", "root execution approval")
    require(tuple(item["path"] for item in lock["files"]) == FILES, "fixed dependency list")
    for item in lock["files"]:
        raw = (BASE / item["path"]).read_bytes()
        require(len(raw) == item["bytes"] and digest(raw) == item["sha256"], "locked source changed")
    return lock


def produce(lock):
    recipe = read("recipe.json")
    require(recipe["version"] == 1, "recipe version")
    inherited = read("inputs/setup-v1.json")["setup"]
    previous = read("inputs/set76-actual-v1.json")
    wallet = read("inputs/wallet-daily-v1.json")
    zero = next(row["binding"] for row in wallet["positives"] if row["fixture"] == "zero256")
    for key in ("root_public_key", "funding_public_key", "funding_script", "derivation_id"):
        require(inherited["wallet_guard"][key] == zero["guard"][key], "independent zero256 wallet identity")
    require(inherited["authority"] == previous["authority"], "unchanged synthetic authority")
    public_crypto = runpy.run_path(str(BASE / "inputs/crypto.py"))
    gym_crypto = runpy.run_path(str(BASE / "inputs/gym-reference.py"))
    original_model = runpy.run_path(str(BASE / "inputs/set76-model.py"))
    for role in ("admission", "publication", "cutoff"):
        seed = hashlib.sha256(("SLUG set74 public synthetic operator " + role + "\x00v1\x00").encode("ascii")).digest()
        require(public_crypto["ed_public"](seed).hex() == inherited["authority"][role + "_key"],
                "published synthetic online role recipe")
    source = copy.deepcopy(inherited["pool"])
    source.update(recipe["source"])
    source_id = digest(recipe["domains"]["source_ascii"].encode("ascii") + canon(source))
    namespace = recipe["namespace_ascii"].encode("ascii")
    require(recipe["accounts"]["published_scalars"] == [1, 2]
            and recipe["accounts"]["labels"] == ["gym1", "gym2"], "selected published gyms")
    accounts = []
    for index, scalar in enumerate(recipe["accounts"]["published_scalars"]):
        pub = public_crypto["pubkey"](scalar)
        require(pub == gym_crypto["public_key"](scalar), "independent public key derivations agree")
        gym = digest(recipe["accounts"]["gym_domain_ascii"].encode("ascii")
                     + struct.pack("<I", source["policy"]["network"])
                     + bytes.fromhex(source["policy"]["genesis"]) + pub)
        require(gym == gym_crypto["gym_id"](source["policy"], pub), "independent gym derivations agree")
        script = (b"\x00\x14" + public_crypto["hash160"](pub)).hex()
        prior = previous["accounts"][index]
        require(prior["published_scalar"] == scalar and prior["public_key"] == pub.hex()
                and prior["gym_id"] == gym and prior["payee_script"] == script,
                "exact Set76 published identities")
        require(script != zero["guard"]["funding_script"], "distinct change and payee")
        accounts.append({"index": index, "label": recipe["accounts"]["labels"][index],
                         "published_scalar": scalar, "public_key": pub.hex(), "gym_id": gym,
                         "payee_script": script,
                         "public_fixture_token": digest(namespace + b"pool-token\x00" + struct.pack("<I", scalar))})
    require(len(accounts) == 2 and len({a["gym_id"] for a in accounts}) == 2, "two independent gyms")
    config = copy.deepcopy(inherited["settlement"])
    config.update(recipe["settlement"])
    config["destinations"] = [{"gym_id": a["gym_id"], "script": a["payee_script"]}
                              for a in sorted(accounts, key=lambda a: a["gym_id"])]
    config_id = framed(recipe["domains"]["config_ascii"], config)
    guard = copy.deepcopy(inherited["wallet_guard"])
    guard["wallet_owner_id"] = digest(namespace + b"wallet-owner")
    guard["source_binding_id"], guard["settlement_config_id"] = source_id, config_id
    binding = {"version": 2, "wallet_owner_id": guard["wallet_owner_id"],
               "operator_profile_id": guard["operator_profile_id"],
               "source": source, "settlement": config, "guard": guard}
    require(guard["wallet_owner_id"] != inherited["wallet_guard"]["wallet_owner_id"]
            and source_id != previous["source_id"] and config_id != previous["config_id"],
            "fresh selected fixture namespace and full bindings")
    capacity = read("inputs/capacity-v1.json")
    capacity_report = read("inputs/capacity-report.json")
    require(capacity_report["passed"] and capacity_report["source_unchanged"]
            and capacity_report["byte_identical"], "independent capacity qualification")
    capacity_raw = (BASE / "inputs/capacity-v1.json").read_bytes()
    require(all(row["exit_code"] == 0 and row["sha256"] == digest(capacity_raw)
                and row["bytes"] == len(capacity_raw) for row in capacity_report["attempts"])
            and len(capacity_report["attempts"]) == 2, "qualified capacity output identity")
    selected = next(row for row in capacity["envelopes"] if row["name"] == recipe["capacity_profile"])
    expected_input = {"source_max": source["max_submissions"], "source_bytes": source["max_bytes"],
                      "rounds": recipe["round_limits"]["max_rounds"], "round_bytes": recipe["round_limits"]["max_bytes"],
                      "credit_rounds": config["max_rounds"], "batches": config["max_batches"], "gyms": len(accounts)}
    require(selected["input"] == expected_input and selected["classification"] == "accept"
            and selected["maximum"] == 2, "actual derived ordinary envelope")
    recovery = next(row for row in capacity["recoveries"] if row["name"] == "kernel-unseen-winner")
    require(recovery["input"] == expected_input and recovery["submissions"] == 3
            and recovery["blocks"] == 2 and recovery["classification"] == "accept", "actual retained N/R bound")
    actual = recipe["actual"]
    require(actual["miner_numerator"] == recipe["kernel"]["miner_numerator"] == 99
            and actual["miner_denominator"] == recipe["kernel"]["miner_denominator"] == 100
            and actual["b_window_shares"] == recipe["kernel"]["b_window_shares"] == [1, 1],
            "fixed independent allocation policy")
    shape = original_model["native_shape"](actual["a_inputs"], actual["a_payees"])
    shape_cases = read("inputs/native84-v1.json")["cases"]
    matching = [row for row in shape_cases if row["inputs"] == 1 and row["payees"] == 1 and row["change"]]
    require(len(matching) == 1 and matching[0]["result"] == "ok"
            and matching[0]["dimensions"] == shape, "independent shape corpus")
    rate = actual["fee_rate_atoms_per_kvb"]
    require(inherited["node"]["fee"]["mode"] == "fixed"
            and inherited["node"]["fee"]["atoms_per_kvb"] == rate, "original selected fee quote")
    fee = (shape["vsize"] * rate + 999) // 1000
    b_reward = actual["reward_atoms"] + fee
    economics = economic_projection(actual["reward_atoms"], b_reward, fee,
                                    actual["b_window_shares"], "actual: exact A included in B")
    kernel = recipe["kernel"]
    kernel_economics = economic_projection(kernel["reward_atoms"], kernel["reward_atoms"], kernel["fee_atoms"],
                                           kernel["b_window_shares"], kernel["scope"])
    require(actual["bootstrap_blocks"] + 1 == actual["a_height"]
            and actual["a_height"] + actual["a_maturity_descendants"] + 1 == actual["b_height"]
            and actual["b_height"] + actual["b_lock_descendants"] == actual["maximum_height"],
            "unchanged real descendant/height sequence")
    require(actual["b_includes_exact_a_transaction"] and not kernel["b_includes_exact_a_transaction"]
            and actual["signed_batches"] == kernel["signed_batches"] == 1,
            "separate scenarios and no second daily slot")
    return {"version": 1, "scope": recipe["scope"], "inputs": lock["files"],
            "authority": inherited["authority"], "binding": binding,
            "source_id": source_id, "config_id": config_id,
            "guard_hex": canon(guard).hex(), "guard_digest": framed(recipe["domains"]["guard_ascii"], guard),
            "binding_hex": canon(binding).hex(), "round_limits": recipe["round_limits"],
            "node": inherited["node"], "accounts": accounts,
            "capacity": {"input": expected_input, "maximum": selected["maximum"],
                         "reserved": selected["reservation"], "actual_recovery": recovery["reservation"]},
            "jobs": recipe["jobs"], "actual_controls": actual, "kernel_controls": kernel,
            "a_shape": shape, "actual_economics": economics, "kernel_economics": kernel_economics,
            "immutable_a": ["day", "created_at_ms", "inputs", "payments", "fee_atoms", "change", "txid", "raw"],
            "mutable_a": ["state only, prepared/broadcast to confirmed"],
            "physical_counts": [{"phase": "A", "jobs": 1, "rows": 1, "accepted": 1, "blocks": 1},
                                {"phase": "ordinary-full", "jobs": 3, "rows": 2, "accepted": 2, "blocks": 1},
                                {"phase": "B-canonical-and-reopened", "jobs": 3, "rows": 3, "accepted": 3, "blocks": 2}]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--lock", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    require(args.out.is_absolute() and not args.out.exists(), "fresh absolute output")
    lock = load_lock(args.lock)
    output = produce(lock)
    load_lock(args.lock)
    raw = json.dumps(output, indent=2, ensure_ascii=True, allow_nan=False).encode("ascii") + b"\n"
    require(len(raw) <= 1 << 20, "bounded public output")
    with args.out.open("xb") as file:
        file.write(raw)
    print(json.dumps({"bytes": len(raw), "sha256": digest(raw), "accounts": len(output["accounts"]),
                      "ordinary_maximum": output["capacity"]["maximum"]}, separators=(",", ":")))


if __name__ == "__main__":
    main()
