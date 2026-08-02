.PHONY: smoke test teacher distill export clean

# venv-managed smoke run (creates .venv, installs deps, runs pytest)
smoke:
	./scripts/smoke.sh

# run pytest with whatever python is active (assumes deps installed)
test:
	python -m pytest tests/ -v --tb=short

# real runs (need datasets + GPU; see README)
teacher:
	python -m liveness_training.teacher.train --config configs/teacher_clip.yaml --out runs/teacher

distill:
	python -m liveness_training.student.distill --config configs/distill.yaml \
		--teacher runs/teacher/teacher_best.pt --out runs/student

export:
	python -m liveness_training.export.onnx_export --student runs/student/student_best.pt \
		--out runs/export

clean:
	rm -rf .venv runs .pytest_cache
	find . -name __pycache__ -type d -exec rm -rf {} +
