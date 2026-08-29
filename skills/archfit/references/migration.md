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
