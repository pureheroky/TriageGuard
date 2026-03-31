"use client";

import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";

const faqs = [
  {
    question: "Do we need to pay before connecting Slack?",
    answer:
      "Yes. Current release is paid-first: activate Team or Enterprise in Billing, then connect Slack and complete onboarding.",
  },
  {
    question: "Will this spam every message in our Slack channels?",
    answer:
      "By default, TriageGuard tracks root messages only in channels you explicitly enabled. You can ignore noise with one click, and SLA reminders stop for ignored requests.",
  },
  {
    question: "How long does setup usually take?",
    answer:
      "Most teams reach first value in about 10 minutes: connect Slack, choose channels, create the first queue, assign triagers and managers, enable an SLA preset, then review open, unacked, unassigned, and at-risk work.",
  },
  {
    question: "Do you support private Slack channels?",
    answer:
      "Yes. TriageGuard supports enabled public and private channels. For private channels, invite the bot explicitly so it can receive events and post triage cards.",
  },
  {
    question: "Do we need Linear to use TriageGuard?",
    answer:
      "No. Linear is optional. TriageGuard already handles triage and SLA directly in Slack. If connected, linked issues sync status, assignee, and due date with Slack requests.",
  },
  {
    question: "What exactly happens when SLA is missed?",
    answer:
      "TriageGuard shows at-risk work before breach, then posts breach and escalation notifications based on queue policy. Waiting and snoozed requests are treated differently from forgotten work to avoid false noise.",
  },
  {
    question: "How does VAT work?",
    answer:
      "Prices are shown in EUR. VAT may apply at checkout depending on customer location.",
  },
  {
    question: "What data is stored?",
    answer:
      "We store the operational metadata needed to control queue health: message text, ownership, queue, request type, due date, waiting or snooze state, SLA clock history, and action history. OAuth tokens are stored server-side and never exposed to browser code.",
  },
  {
    question: "Do you send our data to AI models?",
    answer:
      "No. TriageGuard is deterministic and rule-based for core workflows. No LLM inference is required.",
  },
];

export function FAQ() {
  return (
    <section id="faq" className="border-b border-border/40 py-24 lg:py-32">
      <div className="mx-auto max-w-3xl px-6">
        <div className="mb-16 text-center">
          <p className="mb-3 text-sm font-semibold uppercase tracking-widest text-primary">FAQ</p>
          <h2 className="text-balance text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
            Frequently asked questions
          </h2>
        </div>

        <Accordion type="single" collapsible className="w-full">
          {faqs.map((faq, i) => (
            <AccordionItem key={i} value={`item-${i}`} className="border-border/60">
              <AccordionTrigger className="py-5 text-left text-[15px] font-medium hover:no-underline">
                {faq.question}
              </AccordionTrigger>
              <AccordionContent className="pb-5 text-sm leading-relaxed text-muted-foreground">
                {faq.answer}
              </AccordionContent>
            </AccordionItem>
          ))}
        </Accordion>
      </div>
    </section>
  );
}
