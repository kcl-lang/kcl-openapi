default:
	@echo "pre-processing the k8s swagger.json file"
	python3 main.py swagger.json

gen:
	make
	kcl-openapi generate model -f ./processed-swagger.json