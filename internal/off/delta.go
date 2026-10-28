package off

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// delta is a file of the delta index: the products changed between from and
// to (unix seconds).
type delta struct {
	name     string
	from, to int64
}

var reDeltaName = regexp.MustCompile(`^openfoodfacts_products_(\d+)_(\d+)\.json\.gz$`)

func (s *Syncer) deltaURL(name string) string {
	return strings.TrimSuffix(s.DeltaURL, "/") + "/" + name
}

// fetchIndex reads index.txt: one file name per line, the newest first.
// Lines that are not a delta file are ignored.
func (s *Syncer) fetchIndex(ctx context.Context) ([]delta, error) {
	res, err := s.get(ctx, s.deltaURL("index.txt"))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out []delta
	sc := bufio.NewScanner(io.LimitReader(res.Body, 1<<20))
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		m := reDeltaName.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		from, err1 := strconv.ParseInt(m[1], 10, 64)
		to, err2 := strconv.ParseInt(m[2], 10, 64)
		if err1 == nil && err2 == nil && from < to {
			out = append(out, delta{name: name, from: from, to: to})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read delta index: %w", err)
	}
	return out, nil
}

// pendingDeltas returns, oldest first, the deltas that end after highWater.
// false when they do not reach back to it, or leave a hole between them:
// the changes in between are lost and only a bootstrap recovers them.
func pendingDeltas(index []delta, highWater int64) ([]delta, bool) {
	index = append([]delta(nil), index...)
	sort.Slice(index, func(i, j int) bool { return index[i].from < index[j].from })
	var out []delta
	reach := highWater
	for _, d := range index {
		if d.to <= reach {
			continue
		}
		if d.from > reach {
			return nil, false
		}
		out = append(out, d)
		reach = d.to
	}
	return out, true
}

// applyDelta upserts the Italian products of a delta file and deletes the
// local copies of the ones that are no longer Italian, then advances the
// high-water mark to the file's end. Applying a file twice changes nothing,
// so a failure midway is safe: the next sync applies it again.
func (s *Syncer) applyDelta(ctx context.Context, d delta, stats *SyncStats) error {
	body, err := s.openGzip(ctx, s.deltaURL(d.name))
	if err != nil {
		return err
	}
	defer body.Close()

	now := time.Now().Unix()
	batch := make([]Product, 0, dumpBatchSize)
	flush := func() error {
		removed, err := s.flushBatch(ctx, "off_products", batch, now)
		if err != nil {
			return err
		}
		stats.Removed += removed
		batch = batch[:0]
		return nil
	}
	scanned, err := scanJSONL(ctx, body, func(dp dumpProduct) error {
		if dp.Code == "" {
			return nil
		}
		p := dp.toProduct()
		if !containsTag(dp.CountriesTags, italyTag) || p.Name == "" {
			p = Product{Code: dp.Code} // no name: flushBatch deletes it
		} else {
			stats.Imported++
		}
		batch = append(batch, p)
		if len(batch) >= dumpBatchSize {
			return flush()
		}
		return nil
	})
	stats.ScannedTotal += scanned
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE off_sync_state SET high_water = ?, last_delta_at = ? WHERE id = 1`, d.to, time.Now().Unix()); err != nil {
		return fmt.Errorf("advance high-water mark: %w", err)
	}
	s.Logger.Info("off sync: delta applied", "file", d.name, "scanned", scanned)
	return nil
}
