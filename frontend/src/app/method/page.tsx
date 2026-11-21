import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { MethodPage } from "@/components/op/screens/method-page";

export const metadata: Metadata = {
  title: "How it's built and tested",
  description: "What the model is, what data it sees, how choices were made on development rulings only, how the test sets are kept clean, what didn't work, and how to check it.",
};

export default function Page() {
  return (
    <>
      <AppBar active="method" />
      <MethodPage />
    </>
  );
}
