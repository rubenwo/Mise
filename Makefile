.PHONY: build clean frontend backend

build: frontend backend

frontend:
	cd frontend && npm run build
	rm -rf backend/internal/frontend/dist/*
	cp -r frontend/dist/* backend/internal/frontend/dist/

backend:
	cd backend && go build -buildvcs=false -o server ./cmd/server

clean:
	rm -rf frontend/dist
	rm -rf backend/internal/frontend/dist/*
	touch backend/internal/frontend/dist/.gitkeep
	rm -f backend/server
