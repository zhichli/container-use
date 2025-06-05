all: container-use

container-use:
	@which docker >/dev/null || ( echo "Please follow instructions to install Docker at https://docs.docker.com/get-started/get-docker/"; exit 1 )
	@docker build --platform local -o . .
	@ls container-use

.PHONY: clean
clean:
	rm -f container-use
