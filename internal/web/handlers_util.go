package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"recipe-s3-exporter/internal/storage"
	"recipe-s3-exporter/internal/zerops"
)

// idParam parses the {id} route parameter as int64.
func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// zeropsClientFor loads and decrypts the stored token, returning a client.
func (s *Server) zeropsClientFor(ctx context.Context, tokenID int64) (*zerops.Client, error) {
	tok, err := s.db.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	plain, err := s.cipher.Decrypt(tok.TokenCiphertext)
	if err != nil {
		return nil, err
	}
	return zerops.New(s.cfg.ZeropsAPI, plain), nil
}

// storeForTarget loads and decrypts the target, returning a ready S3 store.
func (s *Server) storeForTarget(ctx context.Context, targetID int64) (*storage.Store, error) {
	t, err := s.db.GetTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	ak, err := s.cipher.Decrypt(t.AccessKeyCiphertext)
	if err != nil {
		return nil, err
	}
	sk, err := s.cipher.Decrypt(t.SecretKeyCiphertext)
	if err != nil {
		return nil, err
	}
	return storage.New(storage.Config{
		Endpoint:     t.Endpoint,
		Region:       t.Region,
		Bucket:       t.Bucket,
		Prefix:       t.Prefix,
		AccessKey:    ak,
		SecretKey:    sk,
		UsePathStyle: t.UsePathStyle,
		UseSSL:       t.UseSSL,
	})
}
