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

export default function ForbiddenPage() {
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Yetkisiz</CardTitle>
          <CardDescription>
            Bu panele erişim için uygun rolünüz yok.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline">
            <Link href={routes.guest.login}>Girişe dön</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
