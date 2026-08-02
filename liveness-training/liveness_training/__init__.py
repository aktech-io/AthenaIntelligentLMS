"""Stage-1 liveness (PAD) training pipeline for Nemo eKYC.

Teacher (domain-generalization, FLIP-style CLIP) -> distilled MobileNetV3-class
student -> ONNX export in the exact deployment shape ekyc-ml-service runs today
(80x80 BGR float32 raw 0..255 face crop behind cv2.dnn).

Plan of record: docs/ekyc/06-level2-upgrade-plan.md §3/§6 and
docs/nemo/09-liveness-build-and-certify.md.
"""

__version__ = "0.1.0"
