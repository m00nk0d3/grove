# Bookmarks

**Bookmark**: a URL the user saved, with a title, the tags given to it, and
the time it was added.

**Tag**: a lower-case label on a bookmark. A bookmark has any number of tags;
listing by tag shows the bookmarks that carry it.

**Store**: the JSON file that holds every bookmark, `~/.bookmarks.json`. It is
read whole and written whole on every command.

**Number**: the position of a bookmark in the last `list` output, counting
from 1. `remove` takes a number.
