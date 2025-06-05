all: install-bin

.PHONY: cu
cu:
	@which docker >/dev/null || ( echo "Please follow instructions to install Docker at https://docs.docker.com/get-started/get-docker/"; exit 1 )
	@docker build --platform local -o . .
	@ls cu

.PHONY: clean
clean:
	rm -f cu

.PHONY: find-path
find-path:
	@PREFERRED_DIR="$$HOME/.local/bin"; \
	if echo "$$PATH" | grep -q "$$PREFERRED_DIR"; then \
		echo "$$PREFERRED_DIR"; \
	else \
		for dir in $$(echo "$$PATH" | tr ':' ' '); do \
			if [ -w "$$dir" ]; then \
				echo "$$dir"; \
				break; \
			fi; \
		done; \
	fi

.PHONY: install-bin
install-bin: container-use
	@DEST=$$(make find-path | tail -n 1); \
	if [ -z "$$DEST" ]; then \
		echo "No writable directory found in \$PATH"; exit 1; \
	fi; \
	echo "Installing container-use to $$DEST..."; \
	mv container-use "$$DEST/"
