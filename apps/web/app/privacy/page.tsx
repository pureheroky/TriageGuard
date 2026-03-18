import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Privacy Policy",
  description: "How TriageGuard collects, uses, and protects workspace data.",
  alternates: {
    canonical: "/privacy",
  },
};

export default function PrivacyPage() {
  return (
    <main className="min-h-screen bg-background">
      <section className="mx-auto max-w-3xl px-6 py-16">
        <p className="text-xs font-semibold uppercase tracking-wide text-primary">Legal</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight text-foreground">TriageGuard Privacy Policy</h1>
        <p className="mt-3 text-sm text-muted-foreground">Effective date: March 3, 2026</p>

        <div className="mt-8 space-y-8 text-sm leading-7 text-muted-foreground">
          <section>
            <h2 className="text-base font-semibold text-foreground">1. Who we are</h2>
            <p>
              TriageGuard is a service for engineering teams to triage Slack requests with ownership, SLA timers, escalations, and optional Linear sync.
              The data controller is TriageGuard.
            </p>
            <p className="mt-2">
              Privacy contact:{" "}
              <a className="text-primary hover:underline" href="mailto:privacy@triageguard.app">
                privacy@triageguard.app
              </a>
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">2. Data we collect</h2>
            <p className="mt-2">Slack data</p>
            <ul className="mt-1 list-disc space-y-1 pl-5">
              <li>Workspace identifiers (`team_id`), channel IDs and channel names.</li>
              <li>User IDs and, when required for assignee mapping, user email from Slack profile.</li>
              <li>Message metadata (`ts`, `thread_ts`) and request links.</li>
              <li>Message content used to create requests (`body_text`).</li>
              <li>Request action history (`ACK`, `ASSIGN`, `PRIORITY`, `DUE`, `CONVERT`, `RESOLVE`, `REOPEN`, `IGNORE`).</li>
            </ul>

            <p className="mt-4">Linear data (if connected)</p>
            <ul className="mt-1 list-disc space-y-1 pl-5">
              <li>OAuth access and refresh tokens.</li>
              <li>Team metadata for team selection.</li>
              <li>Created issue metadata (`issue_id`, `issue_url`, identifier).</li>
            </ul>

            <p className="mt-4">Admin Console data</p>
            <ul className="mt-1 list-disc space-y-1 pl-5">
              <li>Admin auth identity through Supabase Auth (email, user id).</li>
              <li>Session cookies for secure authentication.</li>
              <li>Workspace settings (channels, policies, integrations).</li>
            </ul>

            <p className="mt-4">Billing and technical data</p>
            <ul className="mt-1 list-disc space-y-1 pl-5">
              <li>Stripe customer/subscription identifiers and billing status.</li>
              <li>Request metadata, IP logs, and error logs required for security and debugging.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">3. Why we process this data</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>To provide request intake, card rendering, ownership workflows, SLA reminders, and digest messages.</li>
              <li>To secure the service (signature verification, abuse prevention, audit logging).</li>
              <li>To operate billing and enforce plan limits.</li>
              <li>To provide customer support and troubleshoot incidents.</li>
              <li>To improve product reliability using aggregated usage and error trends.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">4. Legal basis</h2>
            <p>Where applicable, we process personal data under the following bases:</p>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Contract performance (providing TriageGuard to your workspace).</li>
              <li>Legitimate interests (security, fraud prevention, operations, support).</li>
              <li>Consent where explicitly required.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">5. Processors and third parties</h2>
            <p>We use third-party services to deliver TriageGuard, including:</p>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Slack (workspace integration and messaging APIs).</li>
              <li>Linear (optional issue linking and sync).</li>
              <li>Supabase (database and authentication).</li>
              <li>Stripe (billing and subscriptions).</li>
              <li>Cloud infrastructure and logging providers used to host and monitor the service.</li>
            </ul>
            <p className="mt-2">We do not sell customer data.</p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">6. Data location and transfers</h2>
            <p>
              Data is stored by our infrastructure providers and subprocessors in regions they operate. This may involve cross-border transfers depending on
              your selected providers and workspace configuration.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">7. Retention and deletion</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Request records and action logs are retained while your workspace is active.</li>
              <li>Integration tokens are removed when integrations are disconnected, revoked, or workspace data is deleted.</li>
              <li>Billing records are retained as required for accounting and legal obligations.</li>
              <li>Workspace admins can request deletion at any time from the app or via support.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">8. Your rights</h2>
            <p>
              Subject to applicable law, you may request access, correction, deletion, restriction, export, or objection to processing. Send requests to{" "}
              <a className="text-primary hover:underline" href="mailto:privacy@triageguard.app">
                privacy@triageguard.app
              </a>
              .
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">9. Security</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Tokens are stored server-side and can be encrypted at rest with `TOKENS_ENCRYPTION_KEY`.</li>
              <li>Slack request signatures are verified for events and actions.</li>
              <li>Access is restricted to required systems and secure transport (TLS) is used in transit.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">10. Children</h2>
            <p>TriageGuard is a B2B service for workplace use and is not intended for children.</p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">11. Policy updates</h2>
            <p>
              We may update this policy to reflect product or legal changes. Material updates are posted on this page with a new effective date.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">12. Contact</h2>
            <p>
              General support:{" "}
              <a className="text-primary hover:underline" href="mailto:support@triageguard.app">
                support@triageguard.app
              </a>
              <br />
              Security reports:{" "}
              <a className="text-primary hover:underline" href="mailto:security@triageguard.app">
                security@triageguard.app
              </a>
            </p>
          </section>
        </div>

        <div className="mt-10 flex items-center gap-4 text-sm">
          <Link href="/terms" className="font-medium text-primary hover:underline">
            Terms of Use
          </Link>
          <Link href="/support" className="font-medium text-primary hover:underline">
            Support
          </Link>
          <Link href="/" className="font-medium text-primary hover:underline">
            Back to home
          </Link>
        </div>
      </section>
    </main>
  );
}
