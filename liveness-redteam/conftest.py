"""Make ``liveness_redteam`` importable when pytest is invoked from anywhere."""
from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
