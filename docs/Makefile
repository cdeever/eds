# eds-docs - the EdS developer and user guide.
#
# The site lives inside the monorepo, so its GitHub Pages URL is the repository
# name (/eds/) rather than the site name. `make server` mirrors that subpath so
# links behave locally the way they will once deployed.
.PHONY: help html server serve-static clean

default: help

help:
	@echo "eds-docs - available targets"
	@echo
	@echo "  server        local dev server with live reload (:1313)"
	@echo "  html          build the site to public/"
	@echo "  serve-static  build, then serve public/ with python (no live reload)"
	@echo "  clean         remove build artifacts"
	@echo
	@echo "After a fresh clone: git submodule update --init --recursive"

html:
	hugo --minify

server:
	hugo server --bind 0.0.0.0 --baseURL http://localhost:1313/eds/

serve-static: html
	cd public && python3 -m http.server 8000

clean:
	rm -rf public/ resources/ .hugo_build.lock
