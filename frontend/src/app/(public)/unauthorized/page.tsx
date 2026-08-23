import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";

export default function UnauthorizedPage() {
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Oturum gerekli</CardTitle>
          <CardDescription>Devam etmek için giriş yapın.</CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild>
            <Link href={routes.guest.login}>Giriş yap</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
