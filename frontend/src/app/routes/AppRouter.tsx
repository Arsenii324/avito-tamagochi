import { Container, Heading, Stack, Text } from '@chakra-ui/react';
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
        <Stack gap="1" textAlign="center">
          <Heading size="lg">Ави</Heading>
          <Text color="fg.muted">Заботься о питомце — получай бонусы Авито за уровень</Text>
        </Stack>
        <DemoClockPanel />
        <PetPanel />
        <RewardsPanel />
        <LeaderboardPanel />
        <Disclaimer />
      </Stack>
      <DailySummaryModal />
    </Container>
  );
}

// Дисклеймер обязателен, а не украшение: стенд открыт по публичному адресу,
// использует название «Авито» и цвета его логотипа, а награды описаны как
// бонусы на реальные услуги площадки. Без явной оговорки страница читается
// как официальный сервис Авито, которым она не является.
//
// Текст сознательно скучный и конкретный: что это, кем не является и кому
// принадлежат названия. Юридическое лицо не называем — мы его не проверяли,
// а ошибиться в нём хуже, чем не указать.
function Disclaimer() {
  return (
    <Text fontSize="xs" color="fg.muted" textAlign="center" pt="2" pb="4">
      Учебный проект, сделан на хакатоне «Авито Лаборатория кода». Не является официальным сервисом
      Авито, не связан с компанией и не одобрен ею. «Авито» и другие упомянутые названия — товарные
      знаки их правообладателей. Бонусы и награды существуют только внутри этого демо-стенда и не
      дают прав на реальные услуги Авито.
    </Text>
  );
}
