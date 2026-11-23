# Store plugins

A grocery store is connected to Nutreek (#239) with a **plugin**: a small HTTP service that answers three calls. Nutreek ships no plugin, and none for Conad or any other chain: a plugin that reads a store's site may break its terms of use, so who writes or installs one answers for it. This page is the whole contract.

## Setting up

1. Set `NUTREEK_SECRET_KEY` (at least 16 characters, e.g. `openssl rand -hex 32`). It seals the plugins' tokens in the database; without it Settings → Stores says so and connects nothing. Changing it makes the stored tokens unreadable.
2. Open **Settings → Stores → Connect a store** and give a name, the plugin's address and its token. **Test the connection** searches it for «pasta».
3. Put the sources in the order you want. A search starts from the first; each source can be turned off. With no store connected, only Open Food Facts is searched, as before.

By default Nutreek reaches **public https addresses only** (the token travels in a header, so plain http is refused too) (the same guard as the push sender: loopback, private, link-local and shared addresses are refused when the connection is made, redirects are not followed). For a plugin on your own network set `NUTREEK_STORES_ALLOW_PRIVATE=true`, which also allows http: **every account on the instance can then point a store at the private network**, so set it only on an instance you trust. Changing a store's address asks for the token again. The demo has no stores.

## The contract

Every call is a `GET` with `Authorization: Bearer <token>` and an answer of at most 1 MiB in JSON, within 6 seconds.

| Call | Answers |
|---|---|
| `{url}/search?q=text` | `{"products": [product, …]}`: at most 20 are kept |
| `{url}/product?id=ID` | one `product`, whose `id` must be the one asked |
| `{url}/availability?id=A&id=B` | `{"products": [{"id", "price", "available"}, …]}`: up to 50 ids a call; unknown ids are left out |

A `product`:

```json
{
  "id": "8001234",
  "name": "Passata di pomodoro",
  "brand": "Conad",
  "size": {"quantity": 700, "unit": "g"},
  "price": 1.09,
  "currency": "EUR",
  "available": true,
  "unit_weight_g": 120,
  "nutrition": {"kcal": 32, "protein": 1.4, "carbs": 5.1, "fat": 0.2,
                "fiber": 1.2, "sugars": 4.8, "saturated_fat": 0.1, "sodium": 0.01}
}
```

- `id` and `name` are required. `size.unit` is `g`, `ml` or `pz`; `price` is in the currency's unit (default `EUR`); `available` defaults to true.
- `nutrition` is **per 100 g (or ml)** and needs `kcal`, `protein`, `carbs` and `fat`; the others are optional. A product without the four macros is left out of a search, because it cannot become an ingredient (the same rule as for Open Food Facts).
- `unit_weight_g` is the weight of one piece, when the product is counted in pieces.

## What Nutreek does with it

- **Search.** Your own ingredients come first, then the sources in your order. The store's products show with its name and price. A product you already made an ingredient of shows as that ingredient.
- **Picking a product** asks the plugin for `product` again (a form is never trusted), creates an ingredient owned by you with its nutrition, in g (or ml for a `ml` package), and links it to the product (`ingredient_store_products`, with the last price and availability).
- **Shopping list.** A row whose ingredient is linked shows the price of whole packages that cover its quantity (700 g of a 500 g package is two). The footer sums them, naming the stores; rows with no link, or whose unit cannot be compared with the package's (g against ml, pieces with no piece weight), have no price and are left out. **Update prices** calls `availability` for the products of the list and stores the new price and availability; a product out of stock is marked. Prices are as old as the last refresh: they are never asked while the page opens.
- **Disconnecting** a store drops its links; the ingredients stay.

## Not in this version

No cart: a `cart` call that sends the list to the store is left for later. The CSV export, the statistics and the REST API do not know about prices.

## Code

`internal/stores` (client, sealing, the price estimate; tested), `internal/db/stores.go` and migration `0048_store_plugins.sql`, `internal/app/stores_handlers.go` and `stores_search.go`, `pages/stores.html`, the chips in `partials/picker_results.html` and `ingredient_results.html`, the price in `pages/shopping_list.html`.
