# Merging changes in this fork

Use squash merges for ordinary feature and repair pull requests. Keep merge
commits available for upstream synchronization: squashing an upstream-sync PR
copies its changes without retaining the upstream commits as ancestors, making
subsequent synchronization harder to review and resolve.

An upstream-sync pull request should therefore use an explicitly reviewed merge
commit. Verify that its two parents are the intended fork base and upstream
head. For an ordinary squash, verify that the landed commit has one parent (the
reviewed target base) and the reviewed content. Update a stale feature branch
and rerun CI before merging; never treat a queued or running check as green.

After either strategy, build and validate the actual landed commit. A squash
has a different identity from the PR head, so a pre-merge build's commit stamp
is not evidence that the merged revision was installed.

This policy applies to this fork. It does not change the upstream project's
merge policy or remove the fork's ability to preserve upstream ancestry.
