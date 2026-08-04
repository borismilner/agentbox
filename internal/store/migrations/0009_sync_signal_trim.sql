-- FR83 slice 3: what retention took, per topic.
--
-- 0008 shipped without this and the gap check was wrong, which a live run found
-- within the hour. The reasoning that failed: "a cursor has fallen off the edge if
-- it is below the oldest surviving signal". Retention is PER TOPIC, so one quiet
-- topic's ancient row keeps the global minimum low while a busy topic's history is
-- trimmed out from under a reader - cursor 1, oldest surviving 1, and signals 2
-- and 3 on the very topic asked about silently gone. Exactly the hole FR61's rule
-- exists to close.
--
-- "Trimmed" cannot be told from "never existed" by looking at what is left, so it
-- is recorded when it happens: the highest sequence deleted from each topic. A
-- reader whose cursor is at or above its topics' watermarks is served a complete
-- batch; one below is told, and told which sequence a complete read starts from.
--
-- Per topic rather than one global number on purpose. The chattiest topic on this
-- machine is agents:<area>, and a global watermark would report a gap to every
-- stale cursor on every unrelated topic - which trains an agent to ignore the one
-- answer here it must never skim.
--
-- The row outlives the topic's signals deliberately: a topic trimmed away
-- entirely by age is the case the old check got most wrong, and it is the one this
-- row is still there to answer.
CREATE TABLE sync_signal_trim (
    topic      TEXT    PRIMARY KEY,
    high_water INTEGER NOT NULL
);
