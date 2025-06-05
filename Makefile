.PHONY: all
all: cu

.PHONY: cu
cu:
	@which docker >/dev/null || ( echo "Please follow instructions to install Docker at https://docs.docker.com/get-started/get-docker/"; exit 1 )
	@docker build --platform local -o . .
	@ls cu

.PHONY: clean
clean:
	rm -f cu
