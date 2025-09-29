.PHONY: tools ffmpeg audio hls seed up down clean demo demo-run

TOOLS=./tools/ffmpeg ./tools/seed

tools:
	mkdir -p bin
	go build -o bin/ffmpeg-tool ./tools/ffmpeg
	go build -o bin/seed ./tools/seed
	go build -o bin/demo ./tools/demo

ffmpeg:
	@which ffmpeg >/dev/null || (echo "ffmpeg not found" && exit 1)

audio: ffmpeg
	mkdir -p assets/input
	test -f assets/input/demo.mp3 || ffmpeg -f lavfi -i sine=frequency=440:duration=30 -c:a libmp3lame -q:a 4 assets/input/demo.mp3 -y >/dev/null 2>&1

hls: ffmpeg tools audio
	mkdir -p assets/hls/demo
	bin/ffmpeg-tool assets/input/demo.mp3 assets/hls/demo "128k,192k"

seed: tools
	bin/seed -in assets/hls -bucket media

up:
	docker compose up -d --build

up-core:
	docker compose up -d --build minio origin edge redis tracker signaling

down:
	docker compose down -v

clean:
	rm -rf bin assets/hls

demo: hls up

demo-run: tools
	TRACKER=http://localhost:8090 SIGNALING=ws://localhost:8091/ws DEMO_SEG=rickroll/128k/index0.m4s bin/demo | sed -n '1,120p'

full-demo: hls up-core demo-run
	@echo "Open http://localhost:8000 and play http://localhost:8081/demo/master.m3u8 or rickroll/master.m3u8"


