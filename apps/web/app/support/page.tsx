import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Support",
  description: "How to contact TriageGuard support, report security issues, and request data deletion.",
  alternates: {
    canonical: "/support",
  },
};

export default function SupportPage() {
  return (
    <main className="min-h-screen bg-background">
      <section className="mx-auto max-w-3xl px-6 py-16">
        <p className="text-xs font-semibold uppercase tracking-wide text-primary">Legal</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight text-foreground">TriageGuard Support</h1>
        <p className="mt-3 text-sm text-muted-foreground">Operational support for onboarding, integrations, billing, and security.</p>

        <div className="mt-8 space-y-8 text-sm leading-7 text-muted-foreground">
          <section>
            <h2 className="text-base font-semibold text-foreground">1. Contact channels</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>
                General support:{" "}
                <a className="text-primary hover:underline" href="mailto:support@triageguard.app">
                  support@triageguard.app
                </a>
              </li>
              <li>
                Security reports:{" "}
                <a className="text-primary hover:underline" href="mailto:security@triageguard.app">
                  security@triageguard.app
                </a>
              </li>
              <li>
                Privacy and data rights:{" "}
                <a className="text-primary hover:underline" href="mailto:privacy@triageguard.app">
                  privacy@triageguard.app
                </a>
              </li>
            </ul>
            <p className="mt-2">Typical first response target: 24-48 business hours (Europe/Madrid time).</p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">2. What to include in your ticket</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Slack workspace name and `team_id` if available.</li>
              <li>Affected `channel_id` and, if possible, Slack thread URL.</li>
              <li>UTC timestamp when the issue happened.</li>
              <li>Request ID from Admin Console (if present).</li>
              <li>Screenshot of the error and exact error text.</li>
              <li>Steps to reproduce.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">3. Common support topics</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              <li>Slack installation and permission scopes.</li>
              <li>Events API and Interactivity callback configuration.</li>
              <li>SLA reminders and digest timing (policy and timezone).</li>
              <li>Linear connection, team mapping, and issue conversion failures.</li>
              <li>Billing, checkout, webhook sync, and subscription status.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">4. Incident communication</h2>
            <p>
              For known service incidents, we provide updates by direct support communication and in-product notices when available. If your workflow is
              blocked, include &quot;production impact&quot; in the subject line.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">5. Security disclosure</h2>
            <p>
              Please report suspected vulnerabilities privately to{" "}
              <a className="text-primary hover:underline" href="mailto:security@triageguard.app">
                security@triageguard.app
              </a>
              . Include reproduction details and avoid public disclosure before remediation.
            </p>
          </section>

          <section>
            <h2 className="text-base font-semibold text-foreground">6. Data deletion requests</h2>
            <p>
              Workspace admins can delete workspace data from Onboarding - Danger Zone or request deletion by email at{" "}
              <a className="text-primary hover:underline" href="mailto:privacy@triageguard.app">
                privacy@triageguard.app
              </a>
              .
            </p>
          </section>
        </div>

        <div className="mt-10 flex items-center gap-4 text-sm">
          <Link href="/privacy" className="font-medium text-primary hover:underline">
            Privacy Policy
          </Link>
          <Link href="/terms" className="font-medium text-primary hover:underline">
            Terms of Use
          </Link>
          <Link href="/" className="font-medium text-primary hover:underline">
            Back to home
          </Link>
        </div>
      </section>
    </main>
  );
}
