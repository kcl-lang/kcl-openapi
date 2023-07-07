default:
	@echo "pre-processing the k8s swagger.json file"
	python3 main.py ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/raw_k8s.json --omit-status --rename=io.k8s=k8s
	cp ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/processed-raw_k8s.json ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/k8s.json

gen:
	make
	kcl-openapi generate model -f ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/processed-raw_k8s.json

test:
	python3 main.py ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/raw_k8s.json --omit-status --rename=io.k8s=k8s
	diff ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/k8s.json ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/processed-raw_k8s.json
	exit_code=$$?
	rm ../../pkg/kube_resource/generator/assets/files/api_spec/k8s/processed-raw_k8s.json
	exit ${exit_code}
