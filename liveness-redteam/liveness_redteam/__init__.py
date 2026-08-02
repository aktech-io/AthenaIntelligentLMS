"""liveness-redteam — internal PAD red-team rig for the Nemo eKYC stack.

Evaluates ``ekyc-ml-service``'s liveness endpoint against presentation-attack
batteries and tracks progress toward the certification bars:

* **Level 1** (internal gate before booking iBeta): 0 accepted attacks over
  N>=500 presentations covering every L1 species, BPCER <=15%.
* **Level 2**: worst-species APCER <=1% including 3D masks.

See ``README.md`` for the operator protocol and
``docs/ekyc/06-level2-upgrade-plan.md`` §2/§7 for where this sits in the plan.
"""
from __future__ import annotations

__version__ = "0.1.0"

__all__ = [
    "frames",
    "metrics",
    "report",
    "runner",
    "scorers",
    "session",
    "synth",
    "taxonomy",
]
