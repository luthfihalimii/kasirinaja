import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";

type LoginCardProps = {
  username: string;
  password: string;
  isLoggingIn: boolean;
  notice: string;
  onUsernameChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onSubmit: () => void;
};

export function LoginCard({
  username,
  password,
  isLoggingIn,
  notice,
  onUsernameChange,
  onPasswordChange,
  onSubmit,
}: LoginCardProps) {
  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>Login Kasir</CardTitle>
        <CardDescription>Masuk untuk mengaktifkan terminal POS.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div>
          <label htmlFor="login-username" className="mb-1 block text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
            Username
          </label>
          <Input
            id="login-username"
            name="username"
            autoComplete="username"
            value={username}
            onChange={(event) => onUsernameChange(event.target.value)}
            placeholder="cashier"
          />
        </div>
        <div>
          <label htmlFor="login-password" className="mb-1 block text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
            Password
          </label>
          <Input
            id="login-password"
            name="password"
            autoComplete="current-password"
            type="password"
            value={password}
            onChange={(event) => onPasswordChange(event.target.value)}
            placeholder="••••••••"
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                onSubmit();
              }
            }}
          />
        </div>
        {notice ? (
          <p aria-live="polite" className="rounded-lg bg-[var(--c-panel-soft)] px-3 py-2 text-sm text-[var(--c-text)]">
            {notice}
          </p>
        ) : null}
      </CardContent>
      <CardFooter>
        <Button className="w-full" size="lg" onClick={onSubmit} disabled={isLoggingIn}>
          {isLoggingIn ? "Memproses..." : "Masuk"}
        </Button>
      </CardFooter>
    </Card>
  );
}
