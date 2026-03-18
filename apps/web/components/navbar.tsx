"use client";

import { useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Menu, X } from "lucide-react";
import { TriageGuardLogo } from "@/components/triageguard-logo";

const navLinks = [
  { label: "Features", href: "#features" },
  { label: "How it works", href: "#how-it-works" },
  { label: "Pricing", href: "#pricing" },
  { label: "Security", href: "#security" },
  { label: "FAQ", href: "#faq" },
];

export function Navbar() {
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border/40 bg-background/95 backdrop-blur-xl supports-backdrop-filter:bg-background/60">
      <nav className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
        <a href="/" className="flex items-center gap-2.5">
          <div className="flex size-8 items-center justify-center rounded-lg bg-primary">
            <TriageGuardLogo size={18} className="text-primary-foreground" />
          </div>
          <span className="text-lg font-semibold tracking-tight text-foreground">TriageGuard</span>
        </a>

        <ul className="hidden items-center gap-1 md:flex">
          {navLinks.map((link) => (
            <li key={link.href}>
              <a href={link.href} className="rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
                {link.label}
              </a>
            </li>
          ))}
        </ul>

        <div className="hidden items-center gap-2 md:flex">
          <Link href="/dashboard">
            <Button variant="ghost" size="sm" className="text-muted-foreground">
              Admin Console
            </Button>
          </Link>
          <a href="/login?next=%2Fbilling">
            <Button size="sm" className="rounded-lg px-4 font-medium shadow-sm">
              Start subscription
            </Button>
          </a>
        </div>

        <button
          className="inline-flex items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground md:hidden"
          onClick={() => setMobileOpen(!mobileOpen)}
          aria-label={mobileOpen ? "Close menu" : "Open menu"}
        >
          {mobileOpen ? <X className="size-5" /> : <Menu className="size-5" />}
        </button>
      </nav>

      {mobileOpen && (
        <div className="border-t border-border/40 bg-background px-6 py-4 md:hidden">
          <ul className="flex flex-col gap-1">
            {navLinks.map((link) => (
              <li key={link.href}>
                <a
                  href={link.href}
                  className="block rounded-md px-3 py-2.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  onClick={() => setMobileOpen(false)}
                >
                  {link.label}
                </a>
              </li>
            ))}
          </ul>
          <div className="mt-4 flex flex-col gap-2 border-t border-border/40 pt-4">
            <Link href="/dashboard" onClick={() => setMobileOpen(false)}>
              <Button variant="outline" size="sm" className="w-full rounded-lg">
                Admin Console
              </Button>
            </Link>
            <a href="/login?next=%2Fbilling">
              <Button size="sm" className="w-full rounded-lg font-medium">
                Start subscription
              </Button>
            </a>
          </div>
        </div>
      )}
    </header>
  );
}
