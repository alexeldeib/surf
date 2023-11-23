default: push-all

push-all: (push "server") (push "proxy")

build-all: (build "server") (build "proxy")

push APP: (build APP)
	docker push docker.io/alexeldeib/{{APP}}:latest

build APP:
	docker build --platform linux/amd64 -f images/{{APP}}/Dockerfile . -t alexeldeib/{{APP}}:latest
