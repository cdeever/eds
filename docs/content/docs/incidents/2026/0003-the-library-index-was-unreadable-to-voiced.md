---
title: "INC-0003: The Library Index Was Unreadable to voiced"
weight: 3
---

# INC-0003: The Library Index Was Unreadable to voiced, and Nothing Said So

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Systems** | `eds-voiced` on the services workload; `make -C deploy library` |
| **Severity** | Requests were answered as before the index (ADR-0045) rather than from it; some that it would have answered were not. Nothing wrong played |
| **Status** | Open · {{< inc-status "Hardening" >}} |
| **Times** | UTC |

---

## Summary

CHG-0040 copied the owner's library index to `/var/lib/eds-voiced` with
`install`, which labelled it `var_lib_t`. voiced's container, which podman's
`:Z` gives `container_file_t` and its own categories, could not read it.
voiced took an unreadable index for none, as it was written to, and said
nothing: requests went on being guessed at.

## Impact

From 05:19, every request was answered without the library. "Play album The
Wall by Pink Floyd" was sent as "album The Wall" and missed; the index would
have refused that and found *The Wall*. "Abbey Road by the Beatles" played,
being spelled as the library has it.

## Detection

The owner: "lots of attempts that didn't work." The driver's log showed a
title the index could never have chosen, and `ls -Z` showed the label.

## Timeline

| When | What happened |
|---|---|
| 05:19 | CHG-0040: index installed, `var_lib_t`. |
| 05:23–05:25 | Six requests: one played, one missed, four not read as requests (a misheard "play" before an album by name, and one without "by"). |
| 05:40 | Found by the album name sent; the label seen. |

## Root cause

A file put into a `:Z` volume from outside the container keeps the label
its directory's default policy gives it, not the container's. voiced's own
files there, which it writes, have the container's.

## Contributing factors

- `Choose` ignored the index's error, so the fallback was silent.
- The deploy did not check that voiced could read what it was given.

## Corrective actions

- `make library` gives the index the directory's label
  (`chcon --reference`).
- voiced logs a warning on each request while the index cannot be read.

## Preventive actions

- [ ] voiced says at start whether it has an index, and how many artists.

## Lessons learned

- A fallback that is silent is a failure nobody sees.

## Related

[CHG-0040](/docs/changes/2026/0040-the-owners-library-for-voiced/),
[ADR-0046](/docs/architecture/decisions/0046-requests-are-matched-against-the-owners-library/)
