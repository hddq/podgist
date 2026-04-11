package store

import (
	"context"
	"errors"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetPodcastByURL(ctx context.Context, url string) (*domain.Podcast, error) {
	podcast := &domain.Podcast{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, url, title, description, author, site_url, image_url,
		       etag, last_modified, last_fetched_at, created_at, updated_at
		FROM podcasts
		WHERE url = $1
	`, url).Scan(
		&podcast.ID,
		&podcast.URL,
		&podcast.Title,
		&podcast.Description,
		&podcast.Author,
		&podcast.SiteURL,
		&podcast.ImageURL,
		&podcast.ETag,
		&podcast.LastModified,
		&podcast.LastFetchedAt,
		&podcast.CreatedAt,
		&podcast.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return podcast, nil
}

func (s *Store) PodcastEpisodeExists(ctx context.Context, podcastURL, episodeURL string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM podcast_episodes pe
			JOIN podcasts p ON p.id = pe.podcast_id
			WHERE p.url = $1 AND pe.episode_url = $2
		)
	`, podcastURL, episodeURL).Scan(&exists)
	return exists, err
}

func (s *Store) UpsertPodcastMetadata(ctx context.Context, podcast *domain.Podcast) (int64, error) {
	var podcastID int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO podcasts (
			url, title, description, author, site_url, image_url,
			etag, last_modified, last_fetched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (url) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			author = EXCLUDED.author,
			site_url = EXCLUDED.site_url,
			image_url = EXCLUDED.image_url,
			etag = EXCLUDED.etag,
			last_modified = EXCLUDED.last_modified,
			last_fetched_at = EXCLUDED.last_fetched_at,
			updated_at = now()
		RETURNING id
	`, podcast.URL, podcast.Title, podcast.Description, podcast.Author, podcast.SiteURL,
		podcast.ImageURL, podcast.ETag, podcast.LastModified, podcast.LastFetchedAt,
	).Scan(&podcastID)
	return podcastID, err
}

func (s *Store) UpdatePodcastFetchState(ctx context.Context, podcastURL, etag, lastModified string, fetchedAt time.Time) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE podcasts
		SET etag = $2,
		    last_modified = $3,
		    last_fetched_at = $4,
		    updated_at = now()
		WHERE url = $1
	`, podcastURL, etag, lastModified, fetchedAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UpsertPodcastEpisodes(ctx context.Context, podcastID int64, episodes []domain.PodcastEpisodeMetadata) error {
	if len(episodes) == 0 {
		return nil
	}

	return s.WithTx(ctx, func(tx pgx.Tx) error {
		for _, episode := range episodes {
			_, err := tx.Exec(ctx, `
				INSERT INTO podcast_episodes (
					podcast_id, episode_url, guid, title, description,
					published_at, duration_seconds, mime_type, byte_size
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (podcast_id, episode_url) DO UPDATE SET
					guid = EXCLUDED.guid,
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					published_at = EXCLUDED.published_at,
					duration_seconds = EXCLUDED.duration_seconds,
					mime_type = EXCLUDED.mime_type,
					byte_size = EXCLUDED.byte_size,
					updated_at = now()
			`, podcastID, episode.EpisodeURL, episode.GUID, episode.Title, episode.Description,
				episode.PublishedAt, episode.DurationSeconds, episode.MIMEType, episode.ByteSize,
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) UpsertPodcastWithEpisodes(ctx context.Context, podcast *domain.Podcast, episodes []domain.PodcastEpisodeMetadata) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		var podcastID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO podcasts (
				url, title, description, author, site_url, image_url,
				etag, last_modified, last_fetched_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (url) DO UPDATE SET
				title = EXCLUDED.title,
				description = EXCLUDED.description,
				author = EXCLUDED.author,
				site_url = EXCLUDED.site_url,
				image_url = EXCLUDED.image_url,
				etag = EXCLUDED.etag,
				last_modified = EXCLUDED.last_modified,
				last_fetched_at = EXCLUDED.last_fetched_at,
				updated_at = now()
			RETURNING id
		`, podcast.URL, podcast.Title, podcast.Description, podcast.Author, podcast.SiteURL,
			podcast.ImageURL, podcast.ETag, podcast.LastModified, podcast.LastFetchedAt,
		).Scan(&podcastID)
		if err != nil {
			return err
		}

		for _, episode := range episodes {
			_, err := tx.Exec(ctx, `
				INSERT INTO podcast_episodes (
					podcast_id, episode_url, guid, title, description,
					published_at, duration_seconds, mime_type, byte_size
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (podcast_id, episode_url) DO UPDATE SET
					guid = EXCLUDED.guid,
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					published_at = EXCLUDED.published_at,
					duration_seconds = EXCLUDED.duration_seconds,
					mime_type = EXCLUDED.mime_type,
					byte_size = EXCLUDED.byte_size,
					updated_at = now()
			`, podcastID, episode.EpisodeURL, episode.GUID, episode.Title, episode.Description,
				episode.PublishedAt, episode.DurationSeconds, episode.MIMEType, episode.ByteSize,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) PodcastFetchDue(ctx context.Context, podcastURL string, now time.Time, cooldown time.Duration) (bool, *domain.Podcast, error) {
	podcast, err := s.GetPodcastByURL(ctx, podcastURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil, nil
		}
		return false, nil, err
	}
	if podcast.LastFetchedAt == nil {
		return true, podcast, nil
	}
	return now.Sub(*podcast.LastFetchedAt) >= cooldown, podcast, nil
}
