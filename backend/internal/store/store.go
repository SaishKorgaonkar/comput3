package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Domain types ---

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Wallet    string    `json:"wallet"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID             string    `json:"id"`
	TeamID         string    `json:"team_id"`
	Prompt         string    `json:"prompt"`
	State          string    `json:"state"`
	MerkleRoot     string    `json:"merkle_root"`
	AttestationTx  string    `json:"attestation_tx"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ActionLog persists the complete action log for a session as a JSONB array.
type ActionLog struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	TeamID    string    `json:"team_id"`
	Actions   []byte    `json:"actions"` // JSONB array of agent.Action
	CreatedAt time.Time `json:"created_at"`
}

// Attestation records the EAS attestation submitted at session end.
type Attestation struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"session_id"`
	TxHash         string    `json:"tx_hash"`
	AttestationUID string    `json:"attestation_uid"`
	MerkleRoot     string    `json:"merkle_root"`
	SchemaUID      string    `json:"schema_uid"`
	EASScanURL     string    `json:"eas_scan_url"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProviderRecord records the provider selected for a session.
type ProviderRecord struct {
	Address       string    `json:"address"`
	Endpoint      string    `json:"endpoint"`
	PricePerHour  string    `json:"price_per_hour"` // wei as string
	JobsCompleted int64     `json:"jobs_completed"`
	SelectedAt    time.Time `json:"selected_at"`
	SessionID     string    `json:"session_id"`
}

// Container tracks a provisioned Docker container.
type Container struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	SessionID   string    `json:"session_id"`
	DockerID    string    `json:"docker_id"`
	Name        string    `json:"name"`
	Image       string    `json:"image"`
	Status      string    `json:"status"`
	PortsJSON   string    `json:"ports_json"` // JSON object: containerPort → hostPort
	StoragePath string    `json:"storage_path"`
	CreatedAt   time.Time `json:"created_at"`
}

// Payment records an x402 micro-payment.
type Payment struct {
	ID         int64     `json:"id"`
	Wallet     string    `json:"wallet"`
	SessionID  string    `json:"session_id"`
	AmountUSDC string    `json:"amount_usdc"`
	TxHash     string    `json:"tx_hash"`
	Nonce      string    `json:"nonce"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// --- Store ---

type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store and verifies database connectivity.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Migrate creates all tables idempotently.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS teams (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			wallet     TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS teams_wallet_idx ON teams(wallet) WHERE wallet != '';

		CREATE TABLE IF NOT EXISTS sessions (
			id              TEXT PRIMARY KEY,
			team_id         TEXT NOT NULL REFERENCES teams(id),
			prompt          TEXT NOT NULL,
			state           TEXT NOT NULL DEFAULT 'running',
			merkle_root     TEXT NOT NULL DEFAULT '',
			attestation_tx  TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS action_logs (
			id         BIGSERIAL PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			team_id    TEXT NOT NULL,
			actions    JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS attestations (
			id               BIGSERIAL PRIMARY KEY,
			session_id       TEXT NOT NULL REFERENCES sessions(id),
			tx_hash          TEXT NOT NULL,
			attestation_uid  TEXT NOT NULL DEFAULT '',
			merkle_root      TEXT NOT NULL DEFAULT '',
			schema_uid       TEXT NOT NULL DEFAULT '',
			eas_scan_url     TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS provider_selections (
			id             BIGSERIAL PRIMARY KEY,
			session_id     TEXT NOT NULL REFERENCES sessions(id),
			address        TEXT NOT NULL,
			endpoint       TEXT NOT NULL,
			price_per_hour TEXT NOT NULL DEFAULT '0',
			jobs_completed BIGINT NOT NULL DEFAULT 0,
			selected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS containers (
			id           TEXT PRIMARY KEY,
			team_id      TEXT NOT NULL REFERENCES teams(id),
			session_id   TEXT NOT NULL DEFAULT '',
			docker_id    TEXT NOT NULL DEFAULT '',
			name         TEXT NOT NULL,
			image        TEXT NOT NULL,
			status       TEXT NOT NULL DEFAULT 'running',
			ports_json   TEXT NOT NULL DEFAULT '{}',
			storage_path TEXT NOT NULL DEFAULT '',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS payments (
			id           BIGSERIAL PRIMARY KEY,
			wallet       TEXT NOT NULL,
			session_id   TEXT NOT NULL DEFAULT '',
			amount_usdc  TEXT NOT NULL DEFAULT '0',
			tx_hash      TEXT NOT NULL DEFAULT '',
			nonce        TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'confirmed',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS payments_nonce_idx ON payments(nonce) WHERE nonce != '';
	`)
	return err
}

// --- Teams ---

func (s *Store) CreateTeam(ctx context.Context, t *Team) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO teams (id, name, wallet, created_at) VALUES ($1, $2, $3, $4)`,
		t.ID, t.Name, t.Wallet, t.CreatedAt)
	return err
}

func (s *Store) GetTeam(ctx context.Context, id string) (*Team, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(wallet,''), created_at FROM teams WHERE id = $1`, id)
	var t Team
	if err := row.Scan(&t.ID, &t.Name, &t.Wallet, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetOrCreateTeamByWallet finds or creates the team for a given wallet address.
func (s *Store) GetOrCreateTeamByWallet(ctx context.Context, wallet string) (*Team, error) {
	w := strings.ToLower(wallet)
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(wallet,''), created_at FROM teams WHERE wallet = $1`, w)
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Wallet, &t.CreatedAt)
	if err == nil {
		return &t, nil
	}
	now := time.Now().UTC()
	suffix := w
	if len(w) > 10 {
		suffix = w[2:10]
	}
	t = Team{
		ID:        "team-" + suffix,
		Name:      "account-" + suffix,
		Wallet:    w,
		CreatedAt: now,
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO teams (id, name, wallet, created_at) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (wallet) WHERE wallet != '' DO NOTHING`,
		t.ID, t.Name, t.Wallet, t.CreatedAt)
	if err != nil {
		return nil, err
	}
	row = s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(wallet,''), created_at FROM teams WHERE wallet = $1`, w)
	if err2 := row.Scan(&t.ID, &t.Name, &t.Wallet, &t.CreatedAt); err2 != nil {
		return nil, err2
	}
	return &t, nil
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id, team_id, prompt, state, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		sess.ID, sess.TeamID, sess.Prompt, sess.State, sess.CreatedAt, sess.UpdatedAt)
	return err
}

func (s *Store) UpdateSessionState(ctx context.Context, id, state string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET state=$1, updated_at=NOW() WHERE id=$2`, state, id)
	return err
}

func (s *Store) UpdateSessionMerkleRoot(ctx context.Context, id, merkleRoot, attestationTx string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET merkle_root=$1, attestation_tx=$2, updated_at=NOW() WHERE id=$3`,
		merkleRoot, attestationTx, id)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, team_id, prompt, state, merkle_root, attestation_tx, created_at, updated_at
		 FROM sessions WHERE id=$1`, id)
	var sess Session
	err := row.Scan(&sess.ID, &sess.TeamID, &sess.Prompt, &sess.State,
		&sess.MerkleRoot, &sess.AttestationTx, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ListTeamSessions(ctx context.Context, teamID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, prompt, state, merkle_root, attestation_tx, created_at, updated_at
		 FROM sessions WHERE team_id=$1 ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.TeamID, &sess.Prompt, &sess.State,
			&sess.MerkleRoot, &sess.AttestationTx, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, nil
}

// --- Action Logs ---

func (s *Store) UpsertActionLog(ctx context.Context, sessionID, teamID string, actions any) error {
	data, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO action_logs (session_id, team_id, actions)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, sessionID, teamID, data)
	if err != nil {
		// Try update if insert conflicted
		_, err = s.pool.Exec(ctx,
			`UPDATE action_logs SET actions=$1 WHERE session_id=$2`, data, sessionID)
	}
	return err
}

func (s *Store) GetActionLog(ctx context.Context, sessionID string) (*ActionLog, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, session_id, team_id, actions, created_at FROM action_logs WHERE session_id=$1`, sessionID)
	var al ActionLog
	if err := row.Scan(&al.ID, &al.SessionID, &al.TeamID, &al.Actions, &al.CreatedAt); err != nil {
		return nil, err
	}
	return &al, nil
}

// --- Attestations ---

func (s *Store) CreateAttestation(ctx context.Context, a *Attestation) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO attestations (session_id, tx_hash, attestation_uid, merkle_root, schema_uid, eas_scan_url)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		a.SessionID, a.TxHash, a.AttestationUID, a.MerkleRoot, a.SchemaUID, a.EASScanURL)
	return err
}

func (s *Store) GetAttestation(ctx context.Context, sessionID string) (*Attestation, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, session_id, tx_hash, attestation_uid, merkle_root, schema_uid, eas_scan_url, created_at
		 FROM attestations WHERE session_id=$1 ORDER BY id DESC LIMIT 1`, sessionID)
	var a Attestation
	if err := row.Scan(&a.ID, &a.SessionID, &a.TxHash, &a.AttestationUID,
		&a.MerkleRoot, &a.SchemaUID, &a.EASScanURL, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) ListTeamAttestations(ctx context.Context, teamID string) ([]Attestation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.session_id, a.tx_hash, a.attestation_uid, a.merkle_root, a.schema_uid, a.eas_scan_url, a.created_at
		FROM attestations a
		JOIN sessions s ON s.id = a.session_id
		WHERE s.team_id = $1
		ORDER BY a.created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attestation
	for rows.Next() {
		var a Attestation
		if err := rows.Scan(&a.ID, &a.SessionID, &a.TxHash, &a.AttestationUID,
			&a.MerkleRoot, &a.SchemaUID, &a.EASScanURL, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// --- Provider Selections ---

func (s *Store) RecordProviderSelection(ctx context.Context, p *ProviderRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO provider_selections (session_id, address, endpoint, price_per_hour, jobs_completed, selected_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		p.SessionID, p.Address, p.Endpoint, p.PricePerHour, p.JobsCompleted, p.SelectedAt)
	return err
}

// --- Containers ---

func (s *Store) CreateContainer(ctx context.Context, c *Container) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO containers (id, team_id, session_id, docker_id, name, image, status, ports_json, storage_path, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		c.ID, c.TeamID, c.SessionID, c.DockerID, c.Name, c.Image, c.Status, c.PortsJSON, c.StoragePath, c.CreatedAt)
	return err
}

func (s *Store) UpdateContainerStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE containers SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *Store) ListTeamContainers(ctx context.Context, teamID string) ([]Container, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, session_id, docker_id, name, image, status, ports_json, storage_path, created_at
		 FROM containers WHERE team_id=$1 ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Container
	for rows.Next() {
		var c Container
		if err := rows.Scan(&c.ID, &c.TeamID, &c.SessionID, &c.DockerID,
			&c.Name, &c.Image, &c.Status, &c.PortsJSON, &c.StoragePath, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// --- Payments ---

func (s *Store) CreatePayment(ctx context.Context, p *Payment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO payments (wallet, session_id, amount_usdc, tx_hash, nonce, status)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		p.Wallet, p.SessionID, p.AmountUSDC, p.TxHash, p.Nonce, p.Status)
	return err
}

// PaymentNonceExists returns true if the given nonce has already been recorded.
// Used to prevent replay attacks.
func (s *Store) PaymentNonceExists(ctx context.Context, nonce string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM payments WHERE nonce=$1`, nonce).Scan(&count)
	return count > 0, err
}

func (s *Store) ListPaymentsByWallet(ctx context.Context, wallet string) ([]Payment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, wallet, session_id, amount_usdc, tx_hash, nonce, status, created_at
		 FROM payments WHERE wallet=$1 ORDER BY created_at DESC`, strings.ToLower(wallet))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.Wallet, &p.SessionID, &p.AmountUSDC,
			&p.TxHash, &p.Nonce, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
