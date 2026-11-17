package mcpserver

import (
	"fmt"
	"strings"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
)

// The formatters turn domain values into the compact text the LLM reads.
// The raw values travel separately in structuredContent.payload.

func fmtList[T any](noun string, items []T, one func(T) string) string {
	if len(items) == 0 {
		return "No " + noun + " matched."
	}
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = one(it)
	}
	return fmt.Sprintf("%d %s:\n\n%s", len(items), noun, strings.Join(lines, "\n\n"))
}

func fmtPost(p domain.Post) string {
	return fmt.Sprintf("Post %d (user %d): %s\n%s", p.ID, p.UserID, p.Title, p.Body)
}

func fmtComment(c domain.Comment) string {
	return fmt.Sprintf("Comment %d (post %d) %q from <%s>\n%s", c.ID, c.PostID, c.Name, c.Email, c.Body)
}

func fmtAlbum(a domain.Album) string {
	return fmt.Sprintf("Album %d (user %d): %s", a.ID, a.UserID, a.Title)
}

func fmtPhoto(p domain.Photo) string {
	return fmt.Sprintf("Photo %d (album %d): %s\n  url: %s\n  thumbnail: %s", p.ID, p.AlbumID, p.Title, p.URL, p.ThumbnailURL)
}

func fmtTodo(t domain.Todo) string {
	mark := " "
	if t.Completed {
		mark = "x"
	}
	return fmt.Sprintf("Todo %d (user %d) [%s] %s", t.ID, t.UserID, mark, t.Title)
}

func fmtUser(u domain.User) string {
	return fmt.Sprintf("User %d: %s (@%s)\n  email: %s\n  phone: %s\n  website: %s\n  address: %s %s, %s %s (%s, %s)\n  company: %s - %s",
		u.ID, u.Name, u.Username, u.Email, u.Phone, u.Website,
		u.Address.Street, u.Address.Suite, u.Address.City, u.Address.Zipcode, u.Address.Geo.Lat, u.Address.Geo.Lng,
		u.Company.Name, u.Company.CatchPhrase)
}
