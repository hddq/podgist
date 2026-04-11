package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/store"
	"golang.org/x/sync/singleflight"
)

const (
	podcastFetchCooldown = 2 * time.Hour
	podcastFetchTimeout  = 10 * time.Second
)

type PodcastMetadataService struct {
	store        *store.Store
	logger       *slog.Logger
	client       *http.Client
	fetchTimeout time.Duration
	cooldown     time.Duration
	singleflight singleflight.Group
}

func NewPodcastMetadataService(st *store.Store, logger *slog.Logger) *PodcastMetadataService {
	return NewPodcastMetadataServiceWithClient(
		st,
		logger,
		&http.Client{Timeout: podcastFetchTimeout},
		podcastFetchTimeout,
		podcastFetchCooldown,
	)
}

func NewPodcastMetadataServiceWithClient(
	st *store.Store,
	logger *slog.Logger,
	client *http.Client,
	fetchTimeout time.Duration,
	cooldown time.Duration,
) *PodcastMetadataService {
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	if fetchTimeout == 0 {
		fetchTimeout = podcastFetchTimeout
	}
	if cooldown == 0 {
		cooldown = podcastFetchCooldown
	}
	return &PodcastMetadataService{
		store:        st,
		logger:       logger,
		client:       client,
		fetchTimeout: fetchTimeout,
		cooldown:     cooldown,
	}
}

func (s *PodcastMetadataService) ScheduleFetch(reqCtx context.Context, podcastURL string) {
	s.schedule(reqCtx, podcastURL, "", "discover")
}

func (s *PodcastMetadataService) ScheduleRefreshIfEpisodeMissing(reqCtx context.Context, podcastURL, episodeURL string) {
	s.schedule(reqCtx, podcastURL, episodeURL, "missing_episode")
}

func (s *PodcastMetadataService) schedule(reqCtx context.Context, podcastURL, episodeURL, reason string) {
	detachedCtx := context.WithoutCancel(reqCtx)
	go func() {
		ctx, cancel := context.WithTimeout(detachedCtx, s.fetchTimeout)
		defer cancel()

		if episodeURL != "" {
			exists, err := s.store.PodcastEpisodeExists(ctx, podcastURL, episodeURL)
			if err != nil {
				s.logger.Warn("check podcast episode metadata", "podcast_url", podcastURL, "episode_url", episodeURL, "error", err)
				return
			}
			if exists {
				return
			}
		}

		if err := s.fetchIfDue(ctx, podcastURL); err != nil {
			s.logger.Warn("fetch podcast metadata", "podcast_url", podcastURL, "episode_url", episodeURL, "reason", reason, "error", err)
		}
	}()
}

func (s *PodcastMetadataService) fetchIfDue(ctx context.Context, podcastURL string) error {
	due, _, err := s.store.PodcastFetchDue(ctx, podcastURL, time.Now().UTC(), s.cooldown)
	if err != nil || !due {
		return err
	}

	_, err, _ = s.singleflight.Do(podcastURL, func() (any, error) {
		now := time.Now().UTC()
		stillDue, podcast, err := s.store.PodcastFetchDue(ctx, podcastURL, now, s.cooldown)
		if err != nil || !stillDue {
			return nil, err
		}
		return nil, s.fetchAndStore(ctx, podcastURL, podcast, now)
	})
	return err
}

func (s *PodcastMetadataService) fetchAndStore(ctx context.Context, podcastURL string, current *domain.Podcast, now time.Time) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podcastURL, nil)
	if err != nil {
		return err
	}
	if current != nil {
		if current.ETag != "" {
			req.Header.Set("If-None-Match", current.ETag)
		}
		if current.LastModified != "" {
			req.Header.Set("If-Modified-Since", current.LastModified)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		etag := headerOrFallback(resp.Header.Get("ETag"), current, func(p *domain.Podcast) string { return p.ETag })
		lastModified := headerOrFallback(resp.Header.Get("Last-Modified"), current, func(p *domain.Podcast) string { return p.LastModified })
		return s.store.UpdatePodcastFetchState(ctx, podcastURL, etag, lastModified, now)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	feedData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	podcast, episodes, err := parsePodcastFeed(feedData)
	if err != nil {
		return err
	}

	podcast.URL = podcastURL
	podcast.ETag = resp.Header.Get("ETag")
	podcast.LastModified = resp.Header.Get("Last-Modified")
	podcast.LastFetchedAt = &now

	return s.store.UpsertPodcastWithEpisodes(ctx, podcast, episodes)
}

func headerOrFallback(header string, podcast *domain.Podcast, getter func(*domain.Podcast) string) string {
	if header != "" {
		return header
	}
	if podcast == nil {
		return ""
	}
	return getter(podcast)
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Description string    `xml:"description"`
	Link        string    `xml:"link"`
	Image       rssImage  `xml:"image"`
	Author      string    `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author"`
	Owner       rssOwner  `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd owner"`
	ItunesImage rssHref   `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	Items       []rssItem `xml:"item"`
}

type rssOwner struct {
	Name string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd name"`
}

type rssImage struct {
	URL string `xml:"url"`
}

type rssHref struct {
	Href string `xml:"href,attr"`
}

type rssItem struct {
	Title       string       `xml:"title"`
	Description string       `xml:"description"`
	Link        string       `xml:"link"`
	GUID        string       `xml:"guid"`
	PubDate     string       `xml:"pubDate"`
	Enclosure   rssEnclosure `xml:"enclosure"`
	Duration    string       `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length string `xml:"length,attr"`
}

type atomFeed struct {
	XMLName  xml.Name    `xml:"feed"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	Author   atomAuthor  `xml:"author"`
	Links    []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	ID        string     `xml:"id"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Links     []atomLink `xml:"link"`
}

func parsePodcastFeed(data []byte) (*domain.Podcast, []domain.PodcastEpisodeMetadata, error) {
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, nil, err
	}

	switch root.XMLName.Local {
	case "rss":
		return parseRSSFeed(data)
	case "feed":
		return parseAtomFeed(data)
	default:
		return nil, nil, fmt.Errorf("unsupported feed format %q", root.XMLName.Local)
	}
}

func parseRSSFeed(data []byte) (*domain.Podcast, []domain.PodcastEpisodeMetadata, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, nil, err
	}

	podcast := &domain.Podcast{
		Title:       strings.TrimSpace(feed.Channel.Title),
		Description: strings.TrimSpace(feed.Channel.Description),
		Author:      firstNonEmpty(feed.Channel.Author, feed.Channel.Owner.Name),
		SiteURL:     strings.TrimSpace(feed.Channel.Link),
		ImageURL:    firstNonEmpty(feed.Channel.ItunesImage.Href, feed.Channel.Image.URL),
	}

	episodes := make([]domain.PodcastEpisodeMetadata, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		episodeURL := strings.TrimSpace(firstNonEmpty(item.Enclosure.URL, item.Link))
		if episodeURL == "" {
			continue
		}

		publishedAt := parseOptionalTime(item.PubDate)
		durationSeconds := parseDurationSeconds(item.Duration)
		byteSize := parseOptionalInt64(item.Enclosure.Length)

		episodes = append(episodes, domain.PodcastEpisodeMetadata{
			EpisodeURL:      episodeURL,
			GUID:            strings.TrimSpace(item.GUID),
			Title:           strings.TrimSpace(item.Title),
			Description:     strings.TrimSpace(item.Description),
			PublishedAt:     publishedAt,
			DurationSeconds: durationSeconds,
			MIMEType:        strings.TrimSpace(item.Enclosure.Type),
			ByteSize:        byteSize,
		})
	}

	return podcast, episodes, nil
}

func parseAtomFeed(data []byte) (*domain.Podcast, []domain.PodcastEpisodeMetadata, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, nil, err
	}

	podcast := &domain.Podcast{
		Title:       strings.TrimSpace(feed.Title),
		Description: strings.TrimSpace(feed.Subtitle),
		Author:      strings.TrimSpace(feed.Author.Name),
		SiteURL:     atomPrimaryLink(feed.Links),
	}

	episodes := make([]domain.PodcastEpisodeMetadata, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		episodeURL := atomPrimaryLink(entry.Links)
		if episodeURL == "" {
			continue
		}

		publishedAt := parseOptionalTime(firstNonEmpty(entry.Published, entry.Updated))
		description := firstNonEmpty(entry.Summary, entry.Content)

		episodes = append(episodes, domain.PodcastEpisodeMetadata{
			EpisodeURL:  strings.TrimSpace(episodeURL),
			GUID:        strings.TrimSpace(entry.ID),
			Title:       strings.TrimSpace(entry.Title),
			Description: strings.TrimSpace(description),
			PublishedAt: publishedAt,
			MIMEType:    atomLinkType(entry.Links),
		})
	}

	return podcast, episodes, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func parseOptionalTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		ts, err := time.Parse(layout, raw)
		if err == nil {
			utc := ts.UTC()
			return &utc
		}
	}
	return nil
}

func parseDurationSeconds(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return &seconds
	}

	parts := strings.Split(raw, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return nil
	}
	total := 0
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil
		}
		total = total*60 + value
	}
	return &total
}

func parseOptionalInt64(raw string) *int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

func atomPrimaryLink(links []atomLink) string {
	for _, link := range links {
		if strings.TrimSpace(link.Rel) == "enclosure" && strings.TrimSpace(link.Href) != "" {
			return link.Href
		}
	}
	for _, link := range links {
		rel := strings.TrimSpace(link.Rel)
		if (rel == "" || rel == "alternate") && strings.TrimSpace(link.Href) != "" {
			return link.Href
		}
	}
	return ""
}

func atomLinkType(links []atomLink) string {
	for _, link := range links {
		if strings.TrimSpace(link.Rel) == "enclosure" && strings.TrimSpace(link.Type) != "" {
			return strings.TrimSpace(link.Type)
		}
	}
	return ""
}
