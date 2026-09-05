#!/usr/bin/env python3
"""A stand-in for SLPA's CMS while it does not yet answer the consolidation and
gate-pass calls.

It replays the sample values from SLPA's own Swagger, so nodes 5 (container
consolidation) and 6 (container gate pass) can be run end to end locally.

    python3 external-integration/slpa/test/cms_stub.py            # listens on :8087

Point the local services config at it (configs/services.docker.json is
gitignored, so this stays local):

    {"id": "slpa", "url": "http://host.docker.internal:8087", "timeout": "30s",
     "auth": {"type": "bearer", "options": {"token": "local-sandbox"}}}

The two sides deliberately carry different numbers, as SLPA does: a cap
container is the real container the terminal pre-advised, a service-order
container is the placeholder the order was priced against. The Swagger sample
shows the same number on both sides; that is an artefact of the example, and
replaying it would let a number-matching bug pass unnoticed.

Test hooks:
  * seal_no "UNPAID"      -> 422 unpaid invoice, exercising the wait branch
  * seal_no "BADCONT"     -> 422 invalid container
  * CONTAINERS env var    -> comma-separated REAL container numbers to pre-advise
                             (default: the two from the Swagger sample)
  * SO_CONTAINERS env var -> comma-separated service-order placeholder numbers
                             (default: DUMY0000001, DUMY0000002, … one per
                             pre-advised container)
"""

import json
import os
import re
import uuid
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs

CONTAINERS = [c.strip() for c in os.environ.get("CONTAINERS", "MSCU8492019,TCLU1234567").split(",") if c.strip()]

# The service-order side: placeholders, never the real container numbers above.
SO_CONTAINERS = [c.strip() for c in os.environ.get("SO_CONTAINERS", "").split(",") if c.strip()]
if not SO_CONTAINERS:
    SO_CONTAINERS = [f"DUMY{i:07d}" for i in range(1, len(CONTAINERS) + 1)]

PORT = int(os.environ.get("PORT", "8087"))

GATE_PASS_PATH = re.compile(r"^/api/nsw/v1/export/service-orders/([^/]+)/generate-gatepass$")
CONSOLIDATION_PATH = "/api/nsw/v1/export/container-consolidation/fcl"
CONSOLIDATION_DELETE_PATH = re.compile(r"^/api/nsw/v1/export/container-consolidation/([^/]+)$")
ECDN_PATH = "/api/nsw/v1/export/ecdn/upload"
SERVICE_ORDER_PATH = "/api/nsw/v1/export/service-orders"

# The containers each declaration's service order was raised for, as the trader
# entered them. Held because the consolidation lookup answers with them: the
# placeholders it offers are the order's own container numbers, not numbers this
# stub invents. Inventing them was why a save could not resolve the placeholder
# side and went out empty.
ordered_containers: dict[str, list[str]] = {}

# Containers this stub has been told are consolidated, per CUSDEC, so a second
# lookup reports them the way the CMS does: paired, with nothing left to do.
#
# Keyed by declaration rather than globally, because the CMS pre-advises
# containers against one: held globally, the first consignment to consolidate
# MSCU8492019 would make every later consignment open with it already paired and
# nothing left to do.
consolidated: dict[str, set[str]] = {}
issued: dict[str, dict] = {}


def slugify(value: str) -> str:
    return re.sub(r"[^a-z0-9]", "", value.lower())


def sqid(prefix: str, cusdec: str, container_no: str) -> str:
    """The CMS's opaque id for one side of a pairing.

    The declaration is folded in because the save call carries only these ids —
    no cusdecno — so it is the id itself that has to say which consignment a
    pairing belongs to.
    """
    return f"{prefix}-{slugify(cusdec)}-{slugify(container_no)}"


def cusdec_of(pair_id: str) -> str:
    """Recovers the declaration a pairing id was issued under."""
    parts = pair_id.split("-", 2)
    return parts[1] if len(parts) > 2 else ""


def so_for(index: int) -> str:
    """The placeholder a pre-advised container is paired with, by position.

    Position is this stub's own convention for generating a plausible service
    order; it is not a pairing anyone can derive, which is exactly why the
    trader chooses.
    """
    if index < len(SO_CONTAINERS):
        return SO_CONTAINERS[index]
    return f"DUMY{index + 1:07d}"


class Handler(BaseHTTPRequestHandler):
    def _send(self, status: int, body: dict) -> None:
        raw = json.dumps(body, indent=2).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _read_json(self) -> dict:
        length = int(self.headers.get("Content-Length") or 0)
        if not length:
            return {}
        try:
            return json.loads(self.rfile.read(length))
        except json.JSONDecodeError:
            return {}

    def log_message(self, fmt, *args):  # noqa: A003 - quieter, one line per call
        print(f"  stub  {self.command} {self.path} -> {fmt % args}")

    def do_GET(self):  # noqa: N802
        url = urlparse(self.path)
        if url.path != CONSOLIDATION_PATH:
            return self._send(404, {"error": {"code": "NOT_FOUND", "message": self.path}})

        cusdec = (parse_qs(url.query).get("cusdecno") or [""])[0]
        if not cusdec:
            return self._send(422, {"openapi": "3.0.3", "status": 0, "error": {
                "code": "VALIDATION", "message": "cusdecno is required",
                "details": {"cusdecno": ["The cusdecno field is required."]}}})

        paired = consolidated.get(slugify(cusdec), set())
        # The order's own containers when this stub has seen the order; the
        # generated placeholders otherwise, so the consolidation step can still
        # be exercised on its own.
        placeholders = ordered_containers.get(slugify(cusdec)) or []
        cap, so = [], []
        for i, container_no in enumerate(CONTAINERS, start=1):
            so_no = placeholders[i - 1] if i - 1 < len(placeholders) else so_for(i - 1)
            cap.append({
                "sqid": sqid("cap", cusdec, container_no),
                "cusdecserial": cusdec,
                "container_no": container_no,
                "container_size": "40",
                "con_status": "FCL",
                "iso_code": "45G1",
                "line_operator": "MSC",
                "vessel_name": "MSC Maya",
                "vessel_voyage": "V999",
                "port_of_discharge": "LKCMB",
                "vgm": "20000",
                "reefer": "0",
                "container_status": 1,
                "so_container_sqid": sqid("so", cusdec, so_no) if container_no in paired else None,
            })
            so.append({
                "sqid": sqid("so", cusdec, so_no),
                "export_so_id": i,
                "ContainerNumber": so_no,
                "ContainerSize": "40",
                "fcl_sub_lcl_status": 0,
                "Service": 1,
                "tariff_id": 1,
                "total": 100,
                "exchange_rate": 300,
                "lkrtotal": 30000,
            })

        self._send(200, {"openapi": "3.0.3", "status": 1,
                         "data": {"status": 1, "cap_containers": cap, "so_containers": so}})

    def do_DELETE(self):  # noqa: N802
        """Undo a consolidation, so the trader can pair a different container."""
        match = CONSOLIDATION_DELETE_PATH.match(urlparse(self.path).path)
        if not match:
            return self._send(404, {"error": {"code": "NOT_FOUND", "message": self.path}})

        cap_id = match.group(1)
        cusdec = cusdec_of(cap_id)
        removed = False
        for container_no in list(consolidated.get(cusdec, set())):
            if cap_id == sqid("cap", cusdec, container_no):
                consolidated[cusdec].discard(container_no)
                removed = True
        if not removed:
            return self._send(422, {"openapi": "3.0.3", "status": 0, "error": {
                "code": "UNPROCESSABLE", "message": "no consolidation for that container",
                "details": {"cap_id": [f"{cap_id} is not consolidated."]}}})

        print(f"  stub  deleted consolidation {cap_id}")
        return self._send(200, {"openapi": "3.0.3", "status": 1,
                                "message": "Cap container and its consolidation deleted successfully."})

    def do_POST(self):  # noqa: N802
        url = urlparse(self.path)
        body = self._read_json()

        if url.path == ECDN_PATH:
            # The declaration upload answers with a verdict in the nested
            # outcome, not with the envelope's status: "ACCEPTED" is what the
            # integration reads, and Flatten lets data override the envelope.
            # The body arrives as multipart, so nothing is read from it.
            return self._send(200, {"openapi": "3.0.3", "status": 1,
                                    "data": {"status": "ACCEPTED",
                                             "message": "eCDN accepted.",
                                             "reference": f"ECDN-{len(ordered_containers) + 1:06d}",
                                             "validated_at": "2026-09-04T09:00:00+05:30"}})

        if url.path == SERVICE_ORDER_PATH:
            cusdec = slugify(str(body.get("cusdec_no") or ""))
            numbers = [str(c.get("container_no") or "").strip()
                       for c in (body.get("containers") or [])]
            ordered_containers[cusdec] = [n for n in numbers if n]
            slug = str(uuid.uuid4())
            print(f"  stub  service order for {cusdec}: {ordered_containers[cusdec]}")
            return self._send(200, {"openapi": "3.0.3", "status": 1,
                                    "message": "Service order created successfully.",
                                    "data": {"service_order_no": f"SO-FCL-EXPORT-2026-{len(ordered_containers):06d}",
                                             "slug": slug, "status": "pending",
                                             "cusdec_serial": str(body.get("cusdec_no") or ""),
                                             "total_usd": 16, "total_lkr": 4800}})

        if url.path == CONSOLIDATION_PATH:
            pairs = body.get("containers") or []
            if not pairs:
                return self._send(422, {"openapi": "3.0.3", "status": 0, "error": {
                    "code": "VALIDATION", "message": "containers is required",
                    "details": {"containers": ["At least one container pair is required."]}}})
            for pair in pairs:
                cusdec = cusdec_of(str(pair.get("id") or ""))
                for container_no in CONTAINERS:
                    if pair.get("id") == sqid("cap", cusdec, container_no):
                        consolidated.setdefault(cusdec, set()).add(container_no)
            print(f"  stub  consolidated so far: { {k: sorted(v) for k, v in consolidated.items()} }")
            return self._send(200, {"openapi": "3.0.3", "status": 1,
                                    "message": "FCL Container consolidation saved successfully."})

        match = GATE_PASS_PATH.match(url.path)
        if match:
            slug = match.group(1)
            container_no = (body.get("container_no") or "").strip()
            seal_no = (body.get("seal_no") or "").strip()

            if seal_no == "UNPAID":
                return self._send(422, {"openapi": "3.0.3", "status": 0, "error": {
                    "code": "UNPROCESSABLE", "message": "unpaid invoice",
                    "details": {"invoice": ["The service order invoice is unpaid."]}}})
            if seal_no == "BADCONT" or not any(container_no in paired for paired in consolidated.values()):
                return self._send(422, {"openapi": "3.0.3", "status": 0, "error": {
                    "code": "UNPROCESSABLE", "message": "invalid container",
                    "details": {"container_no": [f"{container_no or '(blank)'} is not consolidated for this service order."]}}})

            pass_no = f"GP-2026-{len(issued) + 1:06d}"
            record = {
                "gate_pass_id": len(issued) + 1,
                "gate_pass_no": pass_no,
                "container_no": container_no,
                "truck_no": body.get("truck_no", ""),
                "driver_name": body.get("driver_name", ""),
                "seal_no": seal_no,
                "barcode": (pass_no + container_no).replace("-", ""),
                "status": "ISSUED",
                "issued_at": "2026-08-28T10:25:03.803Z",
                "gate_pass_url": f"https://slpacargoapi.slpa.lk/gate-passes/{pass_no}.pdf",
            }
            issued[pass_no] = record
            print(f"  stub  gate pass {pass_no} for {container_no} on order {slug}")
            return self._send(200, {"openapi": "3.0.3", "status": 1, "data": record})

        self._send(404, {"error": {"code": "NOT_FOUND", "message": self.path}})


if __name__ == "__main__":
    print(f"SLPA CMS stub on :{PORT}")
    print(f"  pre-advised (real):     {CONTAINERS}")
    print(f"  service order (dummy):  {SO_CONTAINERS}")
    HTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
