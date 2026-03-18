import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Terms of Use",
  description: "Terms governing your use of TriageGuard.",
  alternates: {
    canonical: "/terms",
  },
};

export default function TermsPage() {
  return (
    <main className="min-h-screen bg-background">
      <section className="mx-auto max-w-3xl px-6 py-16">
        <p className="text-xs font-semibold uppercase tracking-wide text-primary">Legal</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight text-foreground">TriageGuard Terms of Use</h1>
        <p className="mt-3 text-sm text-muted-foreground">Effective date: March 3, 2026</p>

        <div className="mt-8 space-y-8 text-sm leading-7 text-muted-foreground">
          <section>
            <h2 className="text-base font-semibold text-foreground">1. Acceptance of terms</h2>
            <p>
              By accessing or using TriageGuard, you agree to these Terms. If you use TriageGuard on behalf of an organization, you confirm you are authorized
              to bind that organization.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">2. Service description</h2>
            <p>
              TriageGuard helps teams convert Slack messages into structured requests with ownership, priorities, due dates, SLA reminders, escalation
              notifications, and optional Linear issue linking with status/assignee/due sync.
            </p>
            <p className="mt-2">The service is provided on a best-effort basis and may change over time.</p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">3. Account and workspace responsibilities</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Workspace admins are responsible for app installation, channel selection, and policy configuration.</li>
              <li>You must keep your access credentials secure.</li>
              <li>You agree not to misuse the service, attempt unauthorized access, or violate applicable laws.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">4. Third-party integrations</h2>
            <p>
              TriageGuard depends on third-party services such as Slack, Linear, Supabase, and Stripe. Their terms and availability also apply. We are not
              responsible for outages or API limitations of third-party platforms.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">5. Billing, subscription, and cancellation</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Paid access is managed through configured billing providers (Stripe and/or PayPal).</li>
              <li>Current pricing, plan details, and checkout terms are shown on the Billing page.</li>
              <li>Subscriptions renew automatically unless canceled.</li>
              <li>Refunds are handled case-by-case unless otherwise required by law.</li>
              <li>Taxes (including VAT) may apply depending on your jurisdiction.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">6. Plan limits and enforcement</h2>
            <p>
              Plan limits are enforced by the backend. Team is limited to 10 active monitored channels. Enterprise supports unlimited channels and additional
              features such as per-channel SLA overrides and CSV reporting exports. If limits are exceeded, certain actions may be blocked until usage is
              reduced or plan terms change.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">7. Customer data and privacy</h2>
            <p>
              You retain ownership of your workspace data. You grant TriageGuard permission to process that data solely to provide and secure the service. See{" "}
              <Link href="/privacy" className="text-primary hover:underline">
                Privacy Policy
              </Link>{" "}
              for details.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">8. Availability and support scope</h2>
            <p>
              TriageGuard may experience downtime, maintenance windows, or degraded performance. Service availability is provided on a best-effort basis unless
              a separate written SLA is agreed.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">9. Disclaimer</h2>
            <p>
              The service is provided &quot;as is&quot; and &quot;as available&quot; without warranties of uninterrupted operation, fitness for a
              particular purpose, or error-free behavior.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">10. Limitation of liability</h2>
            <p>
              To the maximum extent permitted by law, TriageGuard will not be liable for indirect, incidental, special, consequential, or punitive damages.
              Aggregate liability is limited to fees paid by your workspace to TriageGuard in the 3 months before the claim.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">11. Suspension and termination</h2>
            <p>
              We may suspend or terminate access for abuse, security risk, non-payment, or legal violations. You may stop using the service at any time and can
              cancel your subscription in billing settings.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">12. Governing law</h2>
            <p>
              These Terms are governed by the laws of Spain, without prejudice to mandatory consumer protections that may apply in your jurisdiction.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">13. Changes to terms</h2>
            <p>
              We may update these Terms from time to time. The latest version will be posted on this page with an updated effective date.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">14. Contact</h2>
            <p>
              Terms and legal inquiries:{" "}
              <a className="text-primary hover:underline" href="mailto:support@triageguard.app">
                support@triageguard.app
              </a>
            </p>
          </section>
        </div>

        <div className="mt-10 flex items-center gap-4 text-sm">
          <Link href="/privacy" className="font-medium text-primary hover:underline">
            Privacy Policy
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
