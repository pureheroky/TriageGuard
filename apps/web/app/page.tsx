import type { Metadata } from "next";
import { Navbar } from "@/components/navbar";
import { HeroSection } from "@/components/hero-section";
import { HowItWorks } from "@/components/how-it-works";
import { KeyBenefits } from "@/components/key-benefits";
import { UseCases } from "@/components/use-cases";
import { Pricing } from "@/components/pricing";
import { Security } from "@/components/security";
import { FAQ } from "@/components/faq";
import { Footer } from "@/components/footer";
import { LandingReveal } from "@/components/landing-reveal";
import { getSiteUrl, siteDescription, siteName } from "@/lib/seo";

const siteUrl = getSiteUrl();

export const metadata: Metadata = {
  title: "Slack Triage and SLA Management",
  description: siteDescription,
  alternates: {
    canonical: "/",
  },
  openGraph: {
    url: "/",
    title: `${siteName} | Slack Triage and SLA Management`,
    description: siteDescription,
  },
  twitter: {
    title: `${siteName} | Slack Triage and SLA Management`,
    description: siteDescription,
  },
};

export default function HomePage() {
  const softwareApplicationJsonLd = {
    "@context": "https://schema.org",
    "@type": "SoftwareApplication",
    name: siteName,
    applicationCategory: "BusinessApplication",
    operatingSystem: "Web",
    url: siteUrl,
    description: siteDescription,
    offers: {
      "@type": "Offer",
      price: "89",
      priceCurrency: "EUR",
    },
  };

  return (
    <div className="flex min-h-screen flex-col">
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(softwareApplicationJsonLd) }}
      />
      <Navbar />
      <main>
        <LandingReveal delay={0.02} distance={14}>
          <HeroSection />
        </LandingReveal>
        <LandingReveal delay={0.04}>
          <HowItWorks />
        </LandingReveal>
        <LandingReveal delay={0.06}>
          <KeyBenefits />
        </LandingReveal>
        <LandingReveal delay={0.08}>
          <UseCases />
        </LandingReveal>
        <LandingReveal delay={0.1}>
          <Pricing />
        </LandingReveal>
        <LandingReveal delay={0.12}>
          <Security />
        </LandingReveal>
        <LandingReveal delay={0.14}>
          <FAQ />
        </LandingReveal>
      </main>
      <Footer />
    </div>
  );
}
