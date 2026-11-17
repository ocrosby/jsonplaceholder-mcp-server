package mcpserver

// serverInstructions is sent to the client at initialize. Like sun-mcp's, it
// routes questions to tools and states what is NOT available.
const serverInstructions = `JSONPlaceholder MCP serves the fake REST data behind jsonplaceholder.typicode.com: users, posts, comments, albums, photos and todos. The data is static test data, not real people or content.

Pick by question:
- Users: list_users, get_user.
- Posts: list_posts (pass user_id for one user's posts), get_post.
- Comments: list_comments (pass post_id for one post's comments), get_comment.
- Albums: list_albums (pass user_id for one user's albums), get_album.
- Photos: list_photos (pass album_id for one album's photos), get_photo.
- Todos: list_todos (pass user_id and/or completed), get_todo.

Every list tool is paged: page (default 1) and limit (default 20, maximum 100). Photos has 5000 rows, so always filter by album_id. Nested routes such as /posts/1/comments are the filters above, not separate tools.

Not available here: creating, updating or deleting records (the upstream API fakes writes and persists nothing). A "not found" result means no record has that id.`
