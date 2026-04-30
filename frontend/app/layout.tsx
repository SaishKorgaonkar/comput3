import type { Metadata } from "next";
import { Inter, Space_Mono } from "next/font/google";
import "./globals.css";
import { Web3Providers } from "@/components/Web3Providers";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
  fallback: ["Inter Fallback", "sans-serif"],
});

const spaceMono = Space_Mono({
  variable: "--font-space-mono",
  weight: ["400", "700"],
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "COMPUT3 - Trustless Agentic Cloud",
  description: "Every cloud provider asks you to trust them. We're the only one that proves you can't.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${spaceMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <Web3Providers>{children}</Web3Providers>
      </body>
    </html>
  );
}
