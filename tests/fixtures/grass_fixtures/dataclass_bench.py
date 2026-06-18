from dataclasses import dataclass


@dataclass
class Bench:
    value: int

    def score(self) -> int:
        return self.value * 2


def aggregate(benches: list[Bench]) -> int:
    return sum(b.score for b in benches)
