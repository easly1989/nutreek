# OpenFoodFacts (OFF) strategy

Moved from CLAUDE.md. Read before touching internal/off, ingredient search or the OFF sync.

All OFF text search is **local**, via SQLite FTS5. The `Syncer`
(`internal/off/dump.go`, `bootstrap.go`, `delta.go`) bootstraps from
OFF's CSV export (~0.8 GB compressed), stream-parses it keeping
Italian products (`countries_tags` contains `en:italy`, ~250k
products) into `off_products_staging`, then swaps that into
`off_products` (and its `off_products_fts` FTS5 index) in
transactions of 5,000 rows with a short pause between them, so no
write lock nears the 5 s `busy_timeout`. Searches keep the old products while the export
downloads and may see a mix during the swap; a failed download
leaves them in place. The swap is refused when more than 1% of the
rows are malformed or staging holds under 80% of the current
products. The state is written after the swap, so a swap that fails
midway bootstraps again. It records the high-water mark, the newest
`last_modified_t` of the export (never later than now). Then,
daily, it reads `delta/index.txt` and applies, oldest first, every
delta (one day of changed products, the dump's JSONL shape) that ends
after the high-water mark: Italian products are upserted, local ones
no longer Italian deleted. Result: searches under 50 ms, no HTTP
calls on the search path, no more 503s.

A sync bootstraps from the CSV again when the deltas no longer reach
the high-water mark (offline for more than ~13 days), when
`off.ParserVersion` differs from the stored one (bump it when a parser
change must reach the imported data), or when the last bootstrap is
older than 30 days (deltas never list deleted products). When the
export is older than every delta, it applies them all anyway and
moves the high-water mark on, rather than bootstrapping again daily.

Why: the public OFF endpoints are not reliable for search-as-you-type.
`cgi/search.pl` has an official rate limit of 10 req/min, 3–12 s
latency and ~20% 503s. Search-a-licious has indexing gaps. The only
fully reliable way is to drop the runtime dependency on OFF search.

Barcode lookup (`OFF.GetByBarcode`) stays live against
`world.openfoodfacts.org/api/v2/product/{code}.json`: a single-product
endpoint, reliable, used only the first time a new product is imported.

Manual trigger: `POST /admin/refresh-off-dump` (needs
`X-Admin-Token`) forces a CSV bootstrap. Current state:
`GET /admin/off-status`.

**First start**: `off_products` is empty; the scheduler starts the
bootstrap in the background. Searches return nothing until it
completes (~10 min, ~0.8 GB of bandwidth).
