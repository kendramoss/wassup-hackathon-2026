import { AppBar } from "@/components/op/chrome";
import { Landing } from "@/components/op/landing/landing";

export default function Home() {
  return (
    <>
      <AppBar active="none" sticky />
      <Landing />
    </>
  );
}
