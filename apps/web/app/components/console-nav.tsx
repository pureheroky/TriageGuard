"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

export function ConsoleNav() {
  const router = useRouter();

  async function logout() {
    await fetch("/api/auth/logout", {
      method: "POST",
    });
    router.push("/login");
  }

  return (
    <nav>
      <Link href="/dashboard">Dashboard</Link>
      <Link href="/onboarding">Onboarding</Link>
      <Link href="/channels">Channels</Link>
      <Link href="/queues">Queues</Link>
      <Link href="/policies">Policies</Link>
      <Link href="/ops">Ops</Link>
      <Link href="/linear">Linear</Link>
      <Link href="/billing">Billing</Link>
      <Link href="/activity">Activity</Link>
      <button className="secondary" onClick={logout} type="button">
        Logout
      </button>
    </nav>
  );
}
