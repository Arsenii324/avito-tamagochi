import { Container, Stack } from '@chakra-ui/react';
import { PetPanel } from '@/features/pet/PetPanel';
import { LeaderboardPanel } from '@/features/leaderboard/LeaderboardPanel';
import { RewardsPanel } from '@/features/rewards/RewardsPanel';
import { DailySummaryModal } from '@/features/summary/DailySummaryModal';
import { DemoClockPanel } from '@/features/demo-clock/DemoClockPanel';

// Один экран, не роутер: авторизации ещё нет (feat/auth не смержена),
// демо-пользователь один и фиксированный (backend/cmd/demo_identity.go) —
// маршрутизировать в этом срезе некуда. Имя файла и функции сохранены
// от заготовки feat/frontend-api-layer, чтобы не разъезжаться с main.tsx.
export default function AppRouter() {
  return (
    <Container maxW="3xl" py="8">
      <Stack gap="6">
        <DemoClockPanel />
        <PetPanel />
        <RewardsPanel />
        <LeaderboardPanel />
      </Stack>
      <DailySummaryModal />
    </Container>
  );
}
