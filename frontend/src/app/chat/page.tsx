import type { Metadata } from "next";
import { FreshChat } from "@/components/chat/chat";

export const metadata: Metadata = { title: "Chat" };

export default function ChatPage() {
  return <FreshChat />;
}
