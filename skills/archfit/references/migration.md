# Manual schema migration

Use this reference only when `archfit` rejects an older config or baseline.
Archfit does not include compatibility loaders or migration commands.

## Config v1 to v2

This is the complete v1-to-v2 transform. Do not round-trip the whole file through
a YAML formatter: that can discard comments and reflow unrelated policy.

1. Back up the file:

   ```sh
   cp .archfit.yaml .archfit.yaml.v1.bak
   ```

2. Change the root schema line from `version: 1` to `version: 2`. Preserve an
   inline comment, if present.

3. In `coupling.gate`, remove `min_band` and `max_drop`. Also remove comment
   lines immediately above those keys when the comments describe the retired
   keys.

4. If `coupling.gate.distributed_monolith` already exists, keep it and do not
   add another copy. Otherwise, insert this stanza where the first retired key
   was:

   ```yaml
   coupling:
     gate:
       distributed_monolith:
         # warn is diagnostic. fail blocks only on seams newly introduced against a
         # comparable reference, so switch it on only after a report-only run shows
         # the seam count you expect.
         mode: warn
         max_new_seams: 0
   ```

5. Do not infer `mode: fail`. The retired scalar gate and the
   `distributed_monolith` rule do not have equivalent semantics. The new rule
   counts newly introduced distributed-monolith seams against a comparable
   reference. Start in `warn`, run a comparison, and promote it only by an owner
   decision.

6. Validate the edited file with a report-only run:

   ```sh
   archfit analyze --config .archfit.yaml --format json >/tmp/archfit-state.json
   ```

   Exit 3 means the config is invalid. Fix that error before replacing the
   backup. A successful `analyze` exits 0 even when the architecture verdict is
   `blocked`; read `verdict` in the JSON instead of treating exit 0 as healthy.

7. Review the report, then remove the backup only when the new config is accepted.

An unversioned file cannot be migrated safely by adding a version blindly. It
might contain a YAML document marker or belong to a different schema. Generate a
fresh v2 file with `archfit config init --output /tmp/.archfit.yaml`, then copy
reviewed policy into it.

## Baseline v1 to v2

Do not rewrite baseline JSON by changing `schema_version`. Baseline v2 stores a
new architecture-state reference and cannot be derived safely from a v1 file.

1. Keep the old file for review:

   ```sh
   cp .archfit-baseline.json .archfit-baseline.v1.json
   ```

2. Run `archfit analyze --config .archfit.yaml --format json` and review every
   active finding. A baseline accepts existing debt; it must not hide a new
   finding.
3. After owner approval, regenerate the current baseline:

   ```sh
   archfit baseline --config .archfit.yaml
   ```

4. Run `archfit check --config .archfit.yaml --format json` and confirm the
   expected finding lifecycles and architecture verdict.

## v2.3.0 architecture guardrails

The guardrails release does not provide a blanket migration that rewrites
accepted debt. Before changing a policy or baseline, review these contracts:

- Waiver entries are validated at config load. Each must name a declared or
  supported synthetic rule, provide scope where the finding has endpoints, and provide
  non-empty `reason`, `approved_by`, and a valid `YYYY-MM-DD` `expires` value.
  Invalid metadata exits `3`; do not fill missing values mechanically. Multiple
  matching waivers are order-independent: an active match wins, and an expired
  match is used only when no active match applies.
  `map/uncovered_path`, `map/dead_rule`, and `map/stale_review` have no edge:
  waive them by their exact rule ID without endpoint selectors. The synthetic
  `bc/coupling_gate` cannot be waived; changes to that gate require an approved
  coupling policy decision.
- A stored baseline without a compatible `measurement_profile`, or without the
  `qualifying_seam_ids` snapshot, cannot support a state/dimension/seam delta.
  The run reports a named `gate_reference` `non_comparable` reason. Keep the old
  file for review and do not make it appear current by editing JSON or changing
  only a schema/version field.
  Every pre-v2.3.0 baseline lacks this profile. Capture an approved replacement
  in the same pinned analyzer image/platform, toolchain and build environment
  as CI; matching repository bytes alone do not establish compatible evidence.
- `comparison` is the report-only `--base` comparison. It is separate from
  `gate_reference`, the persisted baseline used by hard gates and drift. A
  `--base` ref never becomes a baseline migration.
- When evidence is incomplete for an applicable fail-gated rule, read
  `decision.unevaluated_required_rules` (`rule_id`, `reason`) before deciding
  that the rule passed. A non-empty list keeps `check` at exit `2` unless a
  known blocker already makes the hard gate fail.

After reviewing the current report and findings, an owner may intentionally run
`archfit baseline`. Capture skips findings covered by temporary waivers,
including expired waivers, and prints the count; it does not accept them as
permanent debt. Re-run `archfit check --format json` and review the complete
state before committing the new baseline.
