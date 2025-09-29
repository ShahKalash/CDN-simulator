package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	var (
		endpoint = flag.String("endpoint", "127.0.0.1:9000", "MinIO endpoint")
		access   = flag.String("access", "minioadmin", "Access key")
		secret   = flag.String("secret", "minioadmin", "Secret key")
		bucket   = flag.String("bucket", "media", "Bucket name")
		inDir    = flag.String("in", "assets/hls", "Input directory to upload")
		useSSL   = flag.Bool("ssl", false, "Use TLS")
	)
	flag.Parse()

	client, err := minio.New(*endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(*access, *secret, ""),
		Secure: *useSSL,
	})
	if err != nil {
		log.Fatalf("minio client: %v", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, *bucket)
	if err != nil {
		log.Fatalf("bucket exists: %v", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, *bucket, minio.MakeBucketOptions{}); err != nil {
			log.Fatalf("make bucket: %v", err)
		}
	}

	err = filepath.WalkDir(*inDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(*inDir, path)
		contentType := "application/octet-stream"
		if filepath.Ext(path) == ".m3u8" {
			contentType = "application/vnd.apple.mpegurl"
		}
		_, err = client.FPutObject(ctx, *bucket, rel, path, minio.PutObjectOptions{ContentType: contentType})
		if err != nil {
			return fmt.Errorf("upload %s: %w", rel, err)
		}
		log.Printf("uploaded %s", rel)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
