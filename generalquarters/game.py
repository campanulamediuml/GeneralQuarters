"""Simple tactical WWII wargame logic."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List, Optional, Tuple


@dataclass
class Unit:
    """A single combat unit on the battlefield."""

    name: str
    faction: str
    unit_type: str
    health: int = 10
    attack: int = 3
    armor: int = 1
    range: int = 1
    position: Tuple[int, int] = (0, 0)

    def take_damage(self, amount: int) -> int:
        self.health = max(0, self.health - amount)
        return self.health

    @property
    def alive(self) -> bool:
        return self.health > 0


class Battlefield:
    """Very small strategic battle map for a WWII skirmish."""

    def __init__(self, width: int = 8, height: int = 8):
        self.width = width
        self.height = height
        self.units: Dict[str, Unit] = {}

    def add_unit(self, unit: Unit, x: int, y: int) -> Unit:
        unit.position = (x, y)
        self.units[unit.name] = unit
        return unit

    def _distance(self, first: Unit, second: Unit) -> int:
        dx = abs(first.position[0] - second.position[0])
        dy = abs(first.position[1] - second.position[1])
        return max(dx, dy)

    def resolve_attack(self, attacker_name: str, defender_name: str) -> Dict[str, int | str | None]:
        attacker = self.units.get(attacker_name)
        defender = self.units.get(defender_name)

        if attacker is None or defender is None:
            raise ValueError("Both attacker and defender must be valid units.")
        if attacker.faction == defender.faction:
            raise ValueError("Units cannot attack allies.")
        if not attacker.alive or not defender.alive:
            raise ValueError("Only living units can attack or defend.")
        if self._distance(attacker, defender) > attacker.range:
            raise ValueError(f"{attacker_name} is out of range of {defender_name}.")

        damage = max(1, attacker.attack - defender.armor)
        defender.take_damage(damage)
        return {
            "attacker": attacker_name,
            "defender": defender_name,
            "damage": damage,
            "defender_health": defender.health,
        }

    def winner(self) -> Optional[str]:
        factions = {unit.faction for unit in self.units.values() if unit.alive}
        if len(factions) == 1:
            return next(iter(factions))
        if not factions:
            return None
        return None

    def living_units(self) -> List[Unit]:
        return [unit for unit in self.units.values() if unit.alive]


def create_demo_battle() -> Battlefield:
    battlefield = Battlefield(width=8, height=8)

    allies = [
        Unit("Infantry Platoon", "Allies", "infantry", health=12, attack=4, armor=1, range=1, position=(2, 2)),
        Unit("Sherman Tank", "Allies", "tank", health=16, attack=6, armor=2, range=2, position=(1, 1)),
    ]
    axis = [
        Unit("Panzergrenadier", "Axis", "infantry", health=11, attack=4, armor=1, range=1, position=(3, 3)),
        Unit("Tiger Tank", "Axis", "tank", health=18, attack=7, armor=2, range=2, position=(3, 1)),
    ]

    for unit in allies + axis:
        battlefield.add_unit(unit, *unit.position)

    return battlefield


def simulate_battle() -> Dict[str, object]:
    battlefield = create_demo_battle()

    battlefield.resolve_attack("Sherman Tank", "Panzergrenadier")
    battlefield.resolve_attack("Tiger Tank", "Infantry Platoon")
    battlefield.resolve_attack("Infantry Platoon", "Tiger Tank")
    battlefield.resolve_attack("Sherman Tank", "Tiger Tank")

    return {
        "winner": battlefield.winner(),
        "remaining_units": [unit.name for unit in battlefield.living_units()],
        "unit_status": {unit.name: {"health": unit.health, "faction": unit.faction} for unit in battlefield.living_units()},
    }


def main() -> int:
    """Run a quick command-line demonstration of the game."""
    result = simulate_battle()
    print("GeneralQuarters")
    print("Battle Report")
    print(f"Winner: {result['winner'] or 'No victor'}")
    for name, details in result["unit_status"].items():
        print(f"- {name}: {details['health']} HP ({details['faction']})")
    return 0
