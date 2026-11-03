// Package rag implementa búsqueda semántica sobre embeddings (pgvector).
package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	openai "github.com/sashabaranov/go-openai"
)

// Service realiza embeddings y búsqueda por similitud.
type Service struct {
	db         *pgxpool.Pool
	client     *openai.Client
	embedModel string
	threshold  float64
	log        *slog.Logger
}

// New construye el servicio. Si client es nil, las operaciones de embedding se omiten.
func New(db *pgxpool.Pool, client *openai.Client, embedModel string, threshold float64, log *slog.Logger) *Service {
	return &Service{db: db, client: client, embedModel: embedModel, threshold: threshold, log: log}
}

// Embed genera el vector de un texto.
func (s *Service) Embed(ctx context.Context, text string) ([]float32, error) {
	if s.client == nil {
		return nil, fmt.Errorf("rag: cliente OpenAI no configurado")
	}
	resp, err := s.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(s.embedModel),
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("rag: embed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("rag: respuesta de embeddings vacía")
	}
	return resp.Data[0].Embedding, nil
}

// MatchTopic devuelve el code del embedding más similar si supera el umbral.
func (s *Service) MatchTopic(ctx context.Context, text string) (string, float64, error) {
	if s.client == nil || strings.TrimSpace(text) == "" {
		return "", 0, nil
	}
	vec, err := s.Embed(ctx, text)
	if err != nil {
		return "", 0, err
	}
	literal := vectorLiteral(vec)

	var code string
	var score float64
	err = s.db.QueryRow(ctx,
		`SELECT code, 1 - (embedding <=> $1::vector) AS score
		 FROM embeddings ORDER BY embedding <=> $1::vector LIMIT 1`, literal).Scan(&code, &score)
	if err != nil {
		return "", 0, fmt.Errorf("rag: búsqueda: %w", err)
	}
	if score < s.threshold {
		return "", score, nil
	}
	return code, score, nil
}

// Upsert inserta o actualiza un embedding por code.
func (s *Service) Upsert(ctx context.Context, code, topic, content string, vec []float32) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO embeddings (code, topic, content, embedding, model)
		 VALUES ($1, $2, $3, $4::vector, $5)
		 ON CONFLICT (code) DO UPDATE SET topic=EXCLUDED.topic, content=EXCLUDED.content,
		   embedding=EXCLUDED.embedding, model=EXCLUDED.model, updated_at=now()`,
		code, topic, content, vectorLiteral(vec), s.embedModel)
	if err != nil {
		return fmt.Errorf("rag: upsert embedding %q: %w", code, err)
	}
	return nil
}

// Delete elimina el embedding asociado a un code.
func (s *Service) Delete(ctx context.Context, code string) error {
	if _, err := s.db.Exec(ctx, "DELETE FROM embeddings WHERE code = $1", code); err != nil {
		return fmt.Errorf("rag: borrar embedding %q: %w", code, err)
	}
	return nil
}

func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
