/*
	(c) Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sampleFile is two iterations of sampler output: a controller that idles around 100 MiB, restarts
// (the zeros), and comes back heavier in the second iteration.
const sampleFile = `# start,1000
1001,102400
1002,104448
1003,0
1004,106496
# start,2000
2001,102400
2002,0
2003,512000
2004,460800
`

// TestParseCtrlMemSeries pins that the start markers are read as markers and not as samples. They
// share the file with the samples, so a marker parsed as one would read a timestamp as a resident set
// size and report a peak of about 1.6 TiB.
func TestParseCtrlMemSeries(t *testing.T) {
	samples, starts := parseCtrlMemSeries(sampleFile)

	require.Equal(t, []int64{1000, 2000}, starts)
	require.Len(t, samples, 8)
	require.Equal(t, memSample{ts: 1001, rss: 102400}, samples[0])
	require.Equal(t, memSample{ts: 2004, rss: 460800}, samples[7])
}

// TestParseMemSampleRejectsNonSamples pins what does not count as a sample. A torn final line is
// normal, since the file is appended to by a shell loop that can be killed mid-write.
func TestParseMemSampleRejectsNonSamples(t *testing.T) {
	for _, line := range []string{"", "# start,1700000000", "garbage", "1700000000", "abc,123", "123,abc"} {
		_, ok := parseMemSample(line)
		require.False(t, ok, "must not parse [%s] as a sample", line)
	}
}

// TestStatsInDefaultsToTheCurrentIteration pins the window a report defaults to. Samples accumulate
// across iterations in one file, so starting from the first marker rather than the last would report a
// peak from an earlier iteration as if it belonged to this one.
func TestStatsInDefaultsToTheCurrentIteration(t *testing.T) {
	series := testSeries(t, sampleFile)

	stats := series.statsIn(time.Time{}, time.Time{})
	require.Equal(t, 4, stats.samples, "only the samples after the last marker")
	require.Equal(t, int64(512000), stats.peakKb)
	require.Equal(t, int64(460800), stats.lastKb, "last is the newest sample, not the largest")
}

// TestStatsInBoundedWindow pins that a window excludes its end, so the baseline and upgrade windows an
// upgrade report uses cannot both claim the same sample.
func TestStatsInBoundedWindow(t *testing.T) {
	series := testSeries(t, sampleFile)

	base := series.statsIn(time.Unix(1000, 0), time.Unix(2000, 0))
	require.Equal(t, 4, base.samples)
	require.Equal(t, int64(106496), base.peakKb)

	upgrade := series.statsIn(time.Unix(2000, 0), time.Time{})
	require.Equal(t, 4, upgrade.samples)
	require.Equal(t, int64(512000), upgrade.peakKb)

	// the ratio the upgrade check works from: 500 MiB against a 104 MiB baseline
	require.InDelta(t, 4.8, float64(upgrade.peakKb)/float64(base.peakKb), 0.1)
}

// TestStatsInWithoutMarkers pins that samples written before markers existed still report, rather than
// reporting nothing because every line fell outside the window.
func TestStatsInWithoutMarkers(t *testing.T) {
	series := testSeries(t, "1001,102400\n1002,204800\n")

	stats := series.statsIn(time.Time{}, time.Time{})
	require.Equal(t, 2, stats.samples)
	require.Equal(t, int64(204800), stats.peakKb)
}

func testSeries(t *testing.T, out string) *ctrlMemSeries {
	t.Helper()
	samples, starts := parseCtrlMemSeries(out)
	return &ctrlMemSeries{samples: samples, starts: starts}
}
