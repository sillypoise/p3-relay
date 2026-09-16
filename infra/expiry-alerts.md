# Gateway expiry alerts

Owner: Relay maintainer. Scope: the reviewed gateway CA and the two Relay database passwords.
Status: applied on 2026-09-15. Subscription confirmation and provider-side delivery checks were
verified on 2026-09-16; operator confirmation of test notification receipt remains pending.
The private recipient is the existing `budget_alert_email`; do not print it or confirmation links.
`gateway_certificate_expires_at` is public metadata, not secret material. Empty disables these
resources. A nonempty value must be valid UTC RFC3339 and requires the private mailbox.

## Design and limits

Three one-time EventBridge Scheduler jobs publish directly to the Relay SNS operator topic at
30, seven and one day before expiry. No Lambda, application process, polling loop or new queue is
needed. The group-scoped scheduler role can publish only to this topic; CloudWatch has a separate,
resource-scoped publisher authorization for exhausted delivery.

Each invocation permits three retries within one hour. `InvocationDroppedCount` above zero raises
an alarm through the topic. Missing metrics between these three expected invocations are normal,
not a heartbeat guarantee. This detects exhausted Scheduler-to-SNS delivery, not email delivery,
spam filtering, deleted schedules or a wrong expiry supplied by an operator. Inspect expected
invocation metrics at each deadline; missing expected evidence is a defect, not success.

At most 12 publish attempts for three reminders, each below 1 KiB of public metadata, are expected
before operator intervention. There is no application CPU/memory/disk allocation. Scheduler's
one-minute precision and email transport mean notification latency is not a hard deadline guarantee.
The earliest reminder provides maintenance time; complete rotation before expiry, not after an alert.

Completed schedules remain represented in state. Do not auto-delete them and let a later plan
silently recreate past jobs. Updating the identity requires updating this timestamp and checking
that both client trust bundles, gateway identity and database password expiry match the new record.
Existing sessions require explicit draining; expiry alone does not revoke them.

## Apply and verify

1. Compare the expiry with the authenticated public CA and database role metadata. Before first
   apply or rotation, verify all three scheduled dates are in the future. A syntactically valid past
   date is not proof that a reminder will fire; date freshness is an explicit review gate.
2. Run `just infrastructure-test`, then the authenticated `just infrastructure-plan`. Review the
   saved plan, including dates, recipient redaction, role scope and closed compute gates.
3. Apply only that reviewed plan. SNS sends **AWS Notification - Subscription Confirmation**.
   The operator must confirm the `p3-relay-operator-alerts` subscription privately; do not forward
   its link. A created subscription or successful SNS Publish is not inbox-delivery evidence.
4. Verify subscription confirmation through metadata-only output, send a clearly labeled test
   notification, and obtain operator confirmation of receipt. Test the Scheduler publisher path
   separately and inspect its invocation metrics before calling scheduled delivery verified.
5. Confirm the dropped-invocation alarm, TLS-only topic policy, and the three exact schedule inputs.
   Retain runtime activation gates until alert-path verification is complete.

If confirmation remains pending, stop activation rather than silently accepting a disabled alert
path. If delivery fails, investigate the scoped IAM/target/subscription settings; do not broaden
permissions or send credentials in diagnostics. An SNS/topic outage can also prevent the alarm's
email, so this channel is not an independent end-to-end availability guarantee.

## One-time delivery validation

After subscription confirmation, `gateway_alert_validation_at` may request a separate one-time job
in the existing group, using the same restricted publisher role and target topic. Review a future
UTC timestamp explicitly; do not compute a moving date during every plan. It sends a labeled TEST
message without modifying the three production reminders. Retry bounds are identical.

Keep the completed job in state as evidence: it cannot recur and has no idle compute allocation.
Remove it at an explicitly approved infrastructure teardown. Reset its date only for an intentional,
reviewed revalidation (for example, after identity rotation). Empty disables this optional resource;
clearing it after creation requires a reviewed destructive plan.

Verify Scheduler invocation and error metrics, then SNS publication/delivery metrics and recipient
receipt. A controlled `SetAlarmState` test may verify the separate CloudWatch-to-SNS route, labeled
as synthetic; it does not simulate retry exhaustion or prove the failure metric itself. Inspect the
alarm's action history and allow metric evaluation to return it to OK.

## Live validation checkpoint — 2026-09-16

The isolated validation job was applied through a reviewed one-addition plan and scheduled for
2026-09-16T00:15:07Z. Production reminder dates were unchanged. In the bounded observation window:

- Scheduler reported one invocation attempt with no target-error/drop datapoints.
- SNS reported three messages published and three notifications delivered, with zero failures:
  the direct TEST notification, synthetic alarm-route test and scheduled TEST notification.
- Alarm action history recorded successful SNS routing at 2026-09-16T00:11:53Z. Normal metric
  evaluation returned the alarm to OK. The ALARM email's reason explicitly labels the synthetic test;
  no real reminder failure was induced.
- IAM simulation allowed the scheduler role to publish to the operator topic and returned
  `implicitDeny` for an unrelated topic. This is simulation evidence, not a live denied request.
- The subsequent AWS drift plan reported no changes. Main state includes the retained validation
  job (43 resources); no gateway or application compute was started.

These observations verify provider-side invocation and notification handling, not inbox visibility.
Obtain operator receipt confirmation before closing the alert-path activation gate.

## Validation and ownership

`infra/tests/expiry.tftest.hcl` covers disabled configuration, missing mailbox, malformed/non-UTC
expiry, exact 30/seven/one-day boundaries, bounded retries, retained schedules, scoped publisher
trust and failure alarm configuration. These are mocked checks, not live delivery tests.
All resources are Relay-owned under OpenTofu; deletion or disabling requires a reviewed destructive
plan. No existing budget, shared database, gateway deployment or neighboring service is changed.

Sources:
- https://docs.aws.amazon.com/scheduler/latest/UserGuide/managing-targets-universal.html
- https://docs.aws.amazon.com/scheduler/latest/UserGuide/monitoring-cloudwatch.html
- https://docs.aws.amazon.com/scheduler/latest/UserGuide/cross-service-confused-deputy-prevention.html
- https://docs.aws.amazon.com/sns/latest/dg/sns-email-notifications.html
