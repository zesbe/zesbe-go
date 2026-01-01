package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

var (
	// BucketSessions is the bucket for storing sessions
	BucketSessions = []byte("sessions")
	// BucketMessages is the bucket for storing messages
	BucketMessages = []byte("messages")
	// BucketMetadata is the bucket for storing metadata
	BucketMetadata = []byte("metadata")
)

// Message represents a chat message
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Tokens    int       `json:"tokens,omitempty"`
	Model     string    `json:"model,omitempty"`
	Provider  string    `json:"provider,omitempty"`
}

// Session represents a chat session
type Session struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	TotalTokens  int       `json:"total_tokens"`
	WorkingDir   string    `json:"working_dir"`
}

// Stats represents session statistics
type Stats struct {
	TotalSessions   int           `json:"total_sessions"`
	TotalMessages   int           `json:"total_messages"`
	TotalTokens     int           `json:"total_tokens"`
	AverageTokens   float64       `json:"average_tokens"`
	MostUsedModel   string        `json:"most_used_model"`
	SessionDuration time.Duration `json:"session_duration"`
}

// Store manages session persistence
type Store struct {
	db       *bolt.DB
	dbPath   string
	current  *Session
	messages []Message
}

// NewStore creates a new session store
func NewStore(dataDir string) (*Store, error) {
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dataDir = filepath.Join(home, ".zesbe-go", "data")
	}

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "sessions.db")

	// Open BoltDB
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{BucketSessions, BucketMessages, BucketMetadata} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}

	return &Store{
		db:       db,
		dbPath:   dbPath,
		messages: make([]Message, 0),
	}, nil
}

// Close closes the session store
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// NewSession creates a new chat session
func (s *Store) NewSession(provider, model string) (*Session, error) {
	wd, _ := os.Getwd()

	session := &Session{
		ID:         uuid.New().String(),
		Title:      fmt.Sprintf("Chat %s", time.Now().Format("2006-01-02 15:04")),
		Provider:   provider,
		Model:      model,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		WorkingDir: wd,
	}

	// Save to database
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSessions)
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}
		return b.Put([]byte(session.ID), data)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	s.current = session
	s.messages = make([]Message, 0)

	return session, nil
}

// GetSession retrieves a session by ID
func (s *Store) GetSession(id string) (*Session, error) {
	var session Session

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSessions)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session not found: %s", id)
		}
		return json.Unmarshal(data, &session)
	})

	if err != nil {
		return nil, err
	}

	return &session, nil
}

// ListSessions returns all sessions sorted by updated time
func (s *Store) ListSessions(limit int) ([]Session, error) {
	var sessions []Session

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSessions)
		return b.ForEach(func(k, v []byte) error {
			var session Session
			if err := json.Unmarshal(v, &session); err != nil {
				return nil // Skip invalid entries
			}
			sessions = append(sessions, session)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Sort by updated time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

// DeleteSession deletes a session and its messages
func (s *Store) DeleteSession(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// Delete session
		b := tx.Bucket(BucketSessions)
		if err := b.Delete([]byte(id)); err != nil {
			return err
		}

		// Delete associated messages
		mb := tx.Bucket(BucketMessages)
		c := mb.Cursor()
		prefix := []byte(id + ":")
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			if err := mb.Delete(k); err != nil {
				return err
			}
		}

		return nil
	})
}

// AddMessage adds a message to the current session
func (s *Store) AddMessage(role, content string, tokens int) (*Message, error) {
	if s.current == nil {
		return nil, fmt.Errorf("no active session")
	}

	msg := Message{
		ID:        uuid.New().String(),
		SessionID: s.current.ID,
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		Tokens:    tokens,
		Model:     s.current.Model,
		Provider:  s.current.Provider,
	}

	// Save to database
	err := s.db.Update(func(tx *bolt.Tx) error {
		// Save message
		mb := tx.Bucket(BucketMessages)
		key := fmt.Sprintf("%s:%s", msg.SessionID, msg.ID)
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if err := mb.Put([]byte(key), data); err != nil {
			return err
		}

		// Update session
		sb := tx.Bucket(BucketSessions)
		s.current.UpdatedAt = time.Now()
		s.current.MessageCount++
		s.current.TotalTokens += tokens
		sessionData, err := json.Marshal(s.current)
		if err != nil {
			return err
		}
		return sb.Put([]byte(s.current.ID), sessionData)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	s.messages = append(s.messages, msg)

	return &msg, nil
}

// GetMessages retrieves messages for a session
func (s *Store) GetMessages(sessionID string) ([]Message, error) {
	var messages []Message

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMessages)
		c := b.Cursor()
		prefix := []byte(sessionID + ":")

		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}
			messages = append(messages, msg)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by timestamp
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	return messages, nil
}

// LoadSession loads a session and its messages
func (s *Store) LoadSession(id string) error {
	session, err := s.GetSession(id)
	if err != nil {
		return err
	}

	messages, err := s.GetMessages(id)
	if err != nil {
		return err
	}

	s.current = session
	s.messages = messages

	// Change to session's working directory if it exists
	if session.WorkingDir != "" {
		if _, err := os.Stat(session.WorkingDir); err == nil {
			os.Chdir(session.WorkingDir)
		}
	}

	return nil
}

// GetCurrentSession returns the current session
func (s *Store) GetCurrentSession() *Session {
	return s.current
}

// GetCurrentMessages returns messages from the current session
func (s *Store) GetCurrentMessages() []Message {
	return s.messages
}

// UpdateSessionTitle updates the session title
func (s *Store) UpdateSessionTitle(id, title string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSessions)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session not found")
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			return err
		}

		session.Title = title
		session.UpdatedAt = time.Now()

		newData, err := json.Marshal(session)
		if err != nil {
			return err
		}

		return b.Put([]byte(id), newData)
	})
}

// GetStats returns session statistics
func (s *Store) GetStats() (*Stats, error) {
	stats := &Stats{}
	modelCount := make(map[string]int)

	err := s.db.View(func(tx *bolt.Tx) error {
		// Count sessions
		sb := tx.Bucket(BucketSessions)
		sb.ForEach(func(k, v []byte) error {
			stats.TotalSessions++
			var session Session
			if err := json.Unmarshal(v, &session); err == nil {
				stats.TotalTokens += session.TotalTokens
				modelCount[session.Model]++
			}
			return nil
		})

		// Count messages
		mb := tx.Bucket(BucketMessages)
		mb.ForEach(func(k, v []byte) error {
			stats.TotalMessages++
			return nil
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Calculate average
	if stats.TotalMessages > 0 {
		stats.AverageTokens = float64(stats.TotalTokens) / float64(stats.TotalMessages)
	}

	// Find most used model
	maxCount := 0
	for model, count := range modelCount {
		if count > maxCount {
			maxCount = count
			stats.MostUsedModel = model
		}
	}

	return stats, nil
}

// ExportSession exports a session to JSON
func (s *Store) ExportSession(id string) ([]byte, error) {
	session, err := s.GetSession(id)
	if err != nil {
		return nil, err
	}

	messages, err := s.GetMessages(id)
	if err != nil {
		return nil, err
	}

	export := struct {
		Session  *Session  `json:"session"`
		Messages []Message `json:"messages"`
	}{
		Session:  session,
		Messages: messages,
	}

	return json.MarshalIndent(export, "", "  ")
}

// ClearCurrentMessages clears messages from the current session (in memory only)
func (s *Store) ClearCurrentMessages() {
	s.messages = make([]Message, 0)
}
