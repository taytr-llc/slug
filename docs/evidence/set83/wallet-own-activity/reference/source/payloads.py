"""Independent unchanged Set70 payload arithmetic for conditional WIRE2 cases.

Address authority and transaction reconstruction are inherited premises of the
qualified source cases. This leaf checks only their closed shape, bounded
integer accounting and captured ordering; it never manufactures a transaction.
"""

from inherited import (MAX_HEIGHT, MAX_MONEY, MAX_TIME, exact_fields, integer,
                       lower_hex, require, window)


CAPTURE = ("version", "captured_at_ms", "wallet_height", "wallet_tip",
           "node_height", "node_tip", "count", "has_older")
ITEM = ("txid", "raw_sha256", "recorded_at_ms", "coinbase", "block",
        "confirmations", "received_atoms", "spent_atoms", "change_atoms",
        "net_atoms", "fee_atoms", "input_count", "output_count",
        "owned_input_count", "owned_output_count")
BLOCK = ("height", "hash", "time_ms")
OUTPUT = ("index", "atoms", "owned", "change", "address")


def capture_valid(value):
    return (exact_fields(value, CAPTURE) and type(value["version"]) is int and
            value["version"] == 1 and integer(value["captured_at_ms"], 1, MAX_TIME) and
            integer(value["wallet_height"], 0, MAX_HEIGHT) and
            integer(value["node_height"], 0, MAX_HEIGHT) and
            value["node_height"] == value["wallet_height"] and
            lower_hex(value["wallet_tip"], nonzero=True) and
            value["node_tip"] == value["wallet_tip"] and
            integer(value["count"], 0, 256) and type(value["has_older"]) is bool and
            (not value["has_older"] or value["count"] == 256))


def item_valid(value, capture):
    if not exact_fields(value, ITEM) or not capture_valid(capture):
        return False
    if not (lower_hex(value["txid"]) and lower_hex(value["raw_sha256"]) and
            integer(value["recorded_at_ms"], 0, MAX_TIME) and
            type(value["coinbase"]) is bool and
            integer(value["input_count"], 1, 256) and
            integer(value["output_count"], 1, 256) and
            integer(value["owned_input_count"], 0, value["input_count"]) and
            integer(value["owned_output_count"], 0, value["output_count"]) and
            value["owned_input_count"] + value["owned_output_count"] > 0):
        return False
    if not all(integer(value[key], 0, MAX_MONEY)
               for key in ("received_atoms", "spent_atoms", "change_atoms")):
        return False
    if not (integer(value["net_atoms"], -MAX_MONEY, MAX_MONEY) and
            value["net_atoms"] == value["received_atoms"] - value["spent_atoms"] and
            value["change_atoms"] <= value["received_atoms"] and
            (value["owned_input_count"] > 0 or value["spent_atoms"] == 0) and
            (value["owned_output_count"] > 0 or
             value["received_atoms"] == value["change_atoms"] == 0)):
        return False
    unknown_fee = value["coinbase"] or value["owned_input_count"] < value["input_count"]
    if unknown_fee != (value["fee_atoms"] is None):
        return False
    if value["fee_atoms"] is not None and not integer(value["fee_atoms"], 0, value["spent_atoms"]):
        return False
    if value["coinbase"] and not (value["input_count"] == 1 and
                                  value["owned_input_count"] == value["spent_atoms"] == 0):
        return False
    block = value["block"]
    if block is None:
        return integer(value["confirmations"], 0, 0)
    return (exact_fields(block, BLOCK) and
            integer(block["height"], 0, capture["wallet_height"]) and
            lower_hex(block["hash"], nonzero=True) and
            integer(block["time_ms"], 0, MAX_TIME) and
            integer(value["confirmations"], 1, MAX_HEIGHT + 1) and
            value["confirmations"] == capture["wallet_height"] - block["height"] + 1)


def item_order(value):
    block = value["block"]
    return (0, -value["recorded_at_ms"], value["txid"]) if block is None else (
        1, -block["height"], value["txid"])


def page_payload_valid(capture, after, limit, next_after, has_more, items):
    if not capture_valid(capture) or type(items) is not list or type(has_more) is not bool:
        return False
    try:
        expected = window(capture["count"], after, limit)
    except ValueError:
        return False
    if (not integer(next_after, 0, 256) or len(items) != expected["length"] or
            next_after != expected["next"] or has_more != expected["has_more"]):
        return False
    if not all(item_valid(item, capture) for item in items):
        return False
    return (len({item["txid"] for item in items}) == len(items) and
            all(item_order(items[i - 1]) < item_order(items[i]) for i in range(1, len(items))))


def detail_payload_valid(capture, item, outputs, inherited_addresses):
    if (not item_valid(item, capture) or capture["count"] == 0 or
            type(outputs) is not list or len(outputs) != item["output_count"]):
        return False
    total, received, change, owned = 0, 0, 0, 0
    for index, output in enumerate(outputs):
        if not (exact_fields(output, OUTPUT) and integer(output["index"], index, index) and
                integer(output["atoms"], 0, MAX_MONEY) and
                type(output["owned"]) is bool and type(output["change"]) is bool and
                (not output["change"] or output["owned"]) and
                type(output["address"]) is str and output["address"] in inherited_addresses):
            return False
        total += output["atoms"]
        if total > MAX_MONEY:
            return False
        if output["owned"]:
            owned += 1
            received += output["atoms"]
        if output["change"]:
            change += output["atoms"]
    return (owned == item["owned_output_count"] and received == item["received_atoms"] and
            change == item["change_atoms"] and
            (item["fee_atoms"] is None or total + item["fee_atoms"] == item["spent_atoms"]))


def observation_payload_valid(value):
    fields = ("observed_at_ms", "state", "error", "wallet_synced", "wallet_height",
              "wallet_tip", "confirmations", "block_hash")
    if not exact_fields(value, fields) or not integer(value["observed_at_ms"], 1, MAX_TIME):
        return False
    if not (type(value["wallet_synced"]) is bool and
            integer(value["wallet_height"], 0, MAX_HEIGHT) and
            integer(value["confirmations"], 0, MAX_HEIGHT + 1)):
        return False
    if value["state"] == "unavailable":
        return (value["error"] == "wallet_unavailable" and not value["wallet_synced"] and
                value["wallet_height"] == value["confirmations"] == 0 and
                value["wallet_tip"] == value["block_hash"] == "")
    if not (value["state"] in ("not_observed", "unconfirmed", "confirmed") and
            value["error"] == "" and value["wallet_synced"] and
            lower_hex(value["wallet_tip"], nonzero=True)):
        return False
    if value["state"] == "confirmed":
        return (1 <= value["confirmations"] <= value["wallet_height"] + 1 and
                lower_hex(value["block_hash"], nonzero=True))
    return value["confirmations"] == 0 and value["block_hash"] == ""
