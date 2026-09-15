# CusDec submission samples

`submission_accepted.json` is a CusDec payload SLC Edge accepts, kept verbatim as
they supplied it. It is the reference for what Annex A means in practice, and the
answer to "is our payload the right shape".

It is data, not a fixture: no test asserts against it today. Treat it as the
contract to check a change against, and update it only when SLC Edge confirms a
new shape — not to match whatever we happen to send.

## Two things in it that look wrong and are not

- **`transportVoageName`** is misspelled in the specification itself, and SLC
  Edge reads that spelling. `Submission` matches it deliberately. The same is
  true of the CDN payload's `voaygeNumber` and `regYear`, which carry
  `FIXME(slc-edge)` markers in `cdn/submission_payload.go`.
- **`submitter` appears twice** — once inside `properties` and once at the top
  level. Both are in the accepted payload, so `Submission` sends both.

## Placeholders

- **`nswId`** is `REPLACE-WITH-FRESH-NSWID`. The real value is derived, never
  typed: `nswid.For(payload, previousEdgeID)` hashes the payload with the
  previous attempt's edgeId so a retry of one submission repeats and a new
  submission differs. Sending a fixed value would defeat the duplicate
  suppression it exists for.
- **`warehouseCode`, `bol`, `bolSplit`** are the literal string `"string"`, left
  from the sample they were captured in. They are not meaningful values.
