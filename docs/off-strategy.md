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

## Search rules (#224)

A query is split into words (letters and digits; spaces, punctuation and
apostrophes only separate). **Every word must match**, in any order, and a word
may match a different field than the others: «yogurt fage» and «fage yogurt»
both find the Fage yogurt.

- **OFF (`off_products_fts`)**: the index has the product name, the brand and
  the package size (`quantity`, as «500 g»). Each word is a prefix of a word in
  any of them, so «fage 150» finds the 150 g products of Fage. Ranking is bm25
  with the name and brand at weight 1 and the size at 0.5. The same product in
  several sizes is one result unless the query names a size. The index is made
  by migration 0042, which rebuilds it from `off_products` on upgrade (0.6 s
  for 250k rows in a test); the table's triggers keep it in step afterwards.
  The category is not searched here: the dump import does not keep it.
- **Local ingredients (`Ingredients.SearchActive`)**: each word is a substring,
  case-insensitive, of the name, the brand or the name of the category the
  viewer put the ingredient in (categories are per user). There is no package
  size on a local ingredient. At most 8 words count; a query with no letter or
  digit (such as «%») is matched whole, as text, and `%` and `_` are always
  literal.
- **Order of the local results (#225)**: an ingredient whose name is exactly the
  query, then what the viewer used before (rows of their weekly plans and their
  recipes; the most used first, then the most recent), then the other
  ingredients they made, then the ones imported from OpenFoodFacts, each group
  by name. Someone else's use does not count. The picker lists the products of
  the OFF index after these, and leaves out any product the viewer already has as
  an ingredient (matched by barcode, not only among the rows shown), so a
  product and its ingredient appear once.
- Both searches use the same word rule and stay a single indexed query, so the
  latency is the one of a single-word search.

## Imported names

An OFF product is saved as a local ingredient (`source='openfoodfacts'`,
the barcode kept) the first time it is picked or imported, so it is reusable
from then on without OFF. `Ingredients.CreateOFF` normalises the text with
`NormalizeOFFName` / `NormalizeOFFBrand` (`internal/db/ingredient_normalize.go`):

1. whitespace runs collapse into one space, ends trimmed;
2. trailing `, ; : - .` are stripped;
3. a name in all capitals (4+ letters) is lowercased;
4. the first letter is uppercased, the rest is kept as OFF has it;
5. names are cut to 200 runes; a brand list keeps its first brand.

Nutrients and the barcode are never altered.

## Aligned imports (#225)

`ingredients.off_aligned` is 1 while an import is as it was imported: nobody
changed its name, brand, unit, weight per piece or nutrients. `CreateOFF` sets
it and `Ingredients.Update` clears it when a saved value differs (numbers
within 0.01 count as the same, so a form that shows rounded values does not clear
it). A row made by hand never has it. An aligned import is safe to refresh from
its product; a changed one must not be overwritten. Migration 0043 sets it for
existing imports whose product is still in `off_products` with the same four
macros; any other import starts off, the safe side. The ingredient page tells
the two apart.
