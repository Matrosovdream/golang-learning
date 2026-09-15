// Command powbench measures how proof-of-work cost grows with difficulty.
//
//	go run ./cmd/powbench -from 12 -to 24 -runs 5
//
// For each difficulty it mines several different headers and prints the
// average attempts and time next to the expected 2^difficulty attempts.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"hashchain/internal/domain"
)

func main() {
	from := flag.Int("from", 12, "lowest difficulty (bits)")
	to := flag.Int("to", 24, "highest difficulty (bits)")
	runs := flag.Int("runs", 5, "headers mined per difficulty")
	flag.Parse()
	if *from < 0 || *to > 40 || *from > *to || *runs < 1 {
		fmt.Fprintln(os.Stderr, "need 0 <= from <= to <= 40 and runs >= 1")
		os.Exit(2)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "bits\texpected attempts\tavg attempts\tavg time\ttime vs previous\thashrate\t")

	var prevAvg time.Duration
	for bits := *from; bits <= *to; bits++ {
		var attempts uint64
		var took time.Duration
		for run := range *runs {
			// Different DataHash per run → a different, independent search.
			h := domain.Header{
				Height:     uint64(run),
				DataHash:   domain.HashData(fmt.Appendf(nil, "powbench %d/%d", bits, run)),
				Difficulty: uint8(bits),
			}
			start := time.Now()
			_, n, err := domain.Mine(context.Background(), h)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			took += time.Since(start)
			attempts += n
		}

		avg := took / time.Duration(*runs)
		ratio := "-"
		if prevAvg > 0 {
			ratio = fmt.Sprintf("%.1f×", float64(avg)/float64(prevAvg))
		}
		prevAvg = avg
		fmt.Fprintf(tw, "%d\t%d\t%d\t%s\t%s\t%.1f MH/s\t\n",
			bits, uint64(1)<<bits, attempts/uint64(*runs), avg.Round(time.Microsecond), ratio,
			float64(attempts)/took.Seconds()/1e6)
	}
	tw.Flush()
}
