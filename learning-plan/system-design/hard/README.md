# 🔴 Hard — backlog

Not written yet. These are distributed-systems problems where the hard part is correctness under
partitions, not the feature set.

| # | Task | Why hard |
|---|---|---|
| 01 | Distributed key-value store (Dynamo-style) | consistent hashing, vector clocks, quorums, hinted handoff, anti-entropy |
| 02 | Distributed message queue / log (Kafka-style) | partitions, replication, ISR, consumer groups, exactly-once semantics |
| 03 | Google Docs–style collaborative editing | OT vs CRDT, presence, offline merge |
| 04 | Distributed lock / coordination service | leases, fencing tokens, leader election, Raft basics |
| 05 | Global rate limiter & quota system | extends easy 03 across regions: approximate counting, token leasing |
| 06 | Stock exchange / matching engine | order book, determinism, low latency (lessons 46–48), replication |
| 07 | Time-series database | LSM trees, compression, downsampling, out-of-order writes |
| 08 | Ad click aggregation / real-time analytics | stream processing, windows, late events, exactly-once counting |
| 09 | Distributed file system (GFS/HDFS-style) | chunking, master metadata, replication, consistency |
| 10 | Multi-region active-active database | conflict resolution, geo-partitioning, consensus across regions |

Pick one and it gets written as `hard/NN-slug.md` following [`../_template.md`](../_template.md).
