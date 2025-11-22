"""
Immune-Based Edge Placement Algorithm for Hybrid CDN/P2P System
Adapted from Chen et al. (2020) - Edge Server Placement Algorithm

This module implements an artificial immune system algorithm to optimize:
1. Edge server assignment (which peers connect to which edge)
2. Super-peer selection (which peers act as mini-edge caches)
3. Load balancing across edge servers
4. Minimization of access delay (RTT)
"""

import random
import math
import json
import numpy as np
from typing import List, Dict, Tuple, Set
from dataclasses import dataclass, asdict
from collections import defaultdict
import time


@dataclass
class Peer:
    """Represents a peer node in the network"""
    id: str
    region: str
    demand: float  # Traffic demand (kbps or segments/sec)
    rtt_to_edge_a: float  # RTT to Edge A (ms)
    rtt_to_edge_b: float  # RTT to Edge B (ms)
    neighbors: List[str]  # List of neighbor peer IDs


@dataclass
class RTTMatrix:
    """RTT measurements between peers"""
    peer_to_peer: Dict[Tuple[str, str], float]  # (peer_i, peer_j) -> RTT
    peer_to_edge: Dict[Tuple[str, str], float]  # (peer_id, edge_id) -> RTT


@dataclass
class Solution:
    """Represents a candidate solution (antibody)"""
    super_peers: List[str]  # List of peer IDs selected as super peers
    edge_assignments: Dict[str, str]  # peer_id -> 'A' or 'B'
    fitness: float = 0.0
    avg_delay: float = 0.0
    load_imbalance: float = 0.0
    edge_a_load: float = 0.0
    edge_b_load: float = 0.0


class ImmunePlacementAlgorithm:
    """
    Immune-based optimization algorithm for edge placement and super-peer selection
    """
    
    def __init__(
        self,
        peers: List[Peer],
        rtt_matrix: RTTMatrix,
        num_super_peers: int = 10,
        alpha: float = 0.7,  # Weight for delay minimization
        beta: float = 0.3,  # Weight for load balancing
        edge_capacity: float = 10000.0,  # Maximum capacity per edge (kbps)
        pop_size: int = 30,
        clone_factor: int = 3,
        mutation_rate: float = 0.1,
        max_generations: int = 100
    ):
        self.peers = {p.id: p for p in peers}
        self.peer_list = [p.id for p in peers]
        self.rtt_matrix = rtt_matrix
        self.num_super_peers = num_super_peers
        self.alpha = alpha
        self.beta = beta
        self.edge_capacity = edge_capacity
        self.pop_size = pop_size
        self.clone_factor = clone_factor
        self.mutation_rate = mutation_rate
        self.max_generations = max_generations
        
        # Statistics
        self.generation_history = []
        self.best_fitness_history = []
        self.avg_fitness_history = []
    
    def get_peer_rtt(self, peer_i: str, peer_j: str) -> float:
        """Get RTT between two peers"""
        if peer_i == peer_j:
            return 0.0
        key1 = (peer_i, peer_j)
        key2 = (peer_j, peer_i)
        return self.rtt_matrix.peer_to_peer.get(key1, 
                self.rtt_matrix.peer_to_peer.get(key2, 100.0))  # Default 100ms
    
    def get_edge_rtt(self, peer_id: str, edge: str) -> float:
        """Get RTT from peer to edge"""
        key = (peer_id, edge)
        if key in self.rtt_matrix.peer_to_edge:
            return self.rtt_matrix.peer_to_edge[key]
        # Fallback to peer's stored RTT
        peer = self.peers[peer_id]
        return peer.rtt_to_edge_a if edge == 'A' else peer.rtt_to_edge_b
    
    def evaluate_solution(self, solution: Solution) -> Tuple[float, float, float, float, float]:
        """
        Evaluate a solution and compute fitness
        
        Returns:
            (fitness, avg_delay, load_imbalance, edge_a_load, edge_b_load)
        """
        effective_delays = []
        edge_loads = {'A': 0.0, 'B': 0.0}
        
        # For each peer, compute effective delay
        for peer_id, peer in self.peers.items():
            # Option 1: Try super peer (P2P)
            d_super = float('inf')
            if solution.super_peers:
                d_super = min(
                    self.get_peer_rtt(peer_id, sp) 
                    for sp in solution.super_peers
                )
            
            # Option 2: Use assigned edge
            assigned_edge = solution.edge_assignments.get(peer_id, 'A')
            d_edge = self.get_edge_rtt(peer_id, assigned_edge)
            
            # Choose best path (P2P via super peer or direct edge)
            # Add small penalty for super peer to encourage edge use when similar
            d_effective = min(d_super + 5.0, d_edge)  # 5ms penalty for P2P overhead
            
            effective_delays.append((peer_id, d_effective, peer.demand))
            
            # Track edge load (even if served by P2P, track for balance)
            edge_loads[assigned_edge] += peer.demand
        
        # Compute average delay (weighted by demand)
        total_demand = sum(p.demand for p in self.peers.values())
        if total_demand > 0:
            avg_delay = sum(d * r for _, d, r in effective_delays) / total_demand
        else:
            avg_delay = 0.0
        
        # Compute load imbalance (variance)
        load_a = edge_loads['A']
        load_b = edge_loads['B']
        load_avg = (load_a + load_b) / 2.0
        load_imbalance = 0.5 * ((load_a - load_avg)**2 + (load_b - load_avg)**2)
        
        # Normalize for objective function
        # Normalize delay (assume max 200ms)
        normalized_delay = avg_delay / 200.0
        
        # Normalize imbalance (assume max imbalance is when all load on one edge)
        max_imbalance = (total_demand / 2.0)**2
        normalized_imbalance = load_imbalance / max_imbalance if max_imbalance > 0 else 0.0
        
        # Objective function (lower is better)
        objective = self.alpha * normalized_delay + self.beta * normalized_imbalance
        
        # Fitness (higher is better) - inverse of objective
        fitness = 1.0 / (objective + 1e-6)
        
        # Update solution
        solution.fitness = fitness
        solution.avg_delay = avg_delay
        solution.load_imbalance = load_imbalance
        solution.edge_a_load = load_a
        solution.edge_b_load = load_b
        
        return fitness, avg_delay, load_imbalance, load_a, load_b
    
    def generate_random_solution(self) -> Solution:
        """Generate a random initial solution"""
        # Random super peer selection
        super_peers = random.sample(self.peer_list, 
                                   min(self.num_super_peers, len(self.peer_list)))
        
        # Random edge assignments (with some preference for closer edge)
        edge_assignments = {}
        for peer_id, peer in self.peers.items():
            # Prefer closer edge, but add some randomness
            if peer.rtt_to_edge_a < peer.rtt_to_edge_b:
                edge_assignments[peer_id] = 'A' if random.random() < 0.7 else 'B'
            else:
                edge_assignments[peer_id] = 'B' if random.random() < 0.7 else 'A'
        
        solution = Solution(
            super_peers=super_peers,
            edge_assignments=edge_assignments
        )
        self.evaluate_solution(solution)
        return solution
    
    def clone_solution(self, solution: Solution) -> Solution:
        """Create a deep copy of a solution"""
        return Solution(
            super_peers=solution.super_peers.copy(),
            edge_assignments=solution.edge_assignments.copy()
        )
    
    def mutate_solution(self, solution: Solution) -> Solution:
        """Mutate a solution (swap super peers, change edge assignments)"""
        mutated = self.clone_solution(solution)
        
        # Mutate super peers
        if random.random() < self.mutation_rate and len(mutated.super_peers) > 0:
            # Replace one random super peer
            idx = random.randint(0, len(mutated.super_peers) - 1)
            # Choose a peer not already a super peer
            candidates = [p for p in self.peer_list if p not in mutated.super_peers]
            if candidates:
                mutated.super_peers[idx] = random.choice(candidates)
        
        # Mutate edge assignments
        for peer_id in mutated.edge_assignments:
            if random.random() < self.mutation_rate * 0.5:  # Lower rate for edge mutations
                # Switch edge assignment
                mutated.edge_assignments[peer_id] = 'B' if mutated.edge_assignments[peer_id] == 'A' else 'A'
        
        return mutated
    
    def optimize(self) -> Tuple[Solution, List[Dict]]:
        """
        Run the immune-based optimization algorithm
        
        Returns:
            (best_solution, history)
        """
        # Initialize population
        population = [self.generate_random_solution() for _ in range(self.pop_size)]
        
        best_solution = None
        best_fitness = -1.0
        
        print(f"Starting immune-based optimization...")
        print(f"Population size: {self.pop_size}")
        print(f"Super peers: {self.num_super_peers}")
        print(f"Max generations: {self.max_generations}")
        print("-" * 60)
        
        for generation in range(self.max_generations):
            # Evaluate all solutions
            for solution in population:
                self.evaluate_solution(solution)
            
            # Track best solution
            for solution in population:
                if solution.fitness > best_fitness:
                    best_fitness = solution.fitness
                    best_solution = self.clone_solution(solution)
            
            # Sort by fitness (descending)
            population.sort(key=lambda s: s.fitness, reverse=True)
            
            # Statistics
            avg_fitness = sum(s.fitness for s in population) / len(population)
            self.best_fitness_history.append(best_fitness)
            self.avg_fitness_history.append(avg_fitness)
            
            history_entry = {
                'generation': generation,
                'best_fitness': best_fitness,
                'avg_fitness': avg_fitness,
                'best_avg_delay': best_solution.avg_delay,
                'best_load_imbalance': best_solution.load_imbalance,
                'best_edge_a_load': best_solution.edge_a_load,
                'best_edge_b_load': best_solution.edge_b_load
            }
            self.generation_history.append(history_entry)
            
            if generation % 10 == 0:
                print(f"Generation {generation:3d} | Best Fitness: {best_fitness:.4f} | "
                      f"Avg Delay: {best_solution.avg_delay:.2f}ms | "
                      f"Load Imbalance: {best_solution.load_imbalance:.2f}")
            
            # Clonal selection
            # Select top M solutions
            M = max(1, self.pop_size // 3)
            new_population = []
            
            for idx in range(M):
                solution = population[idx]
                fit_ratio = solution.fitness / population[0].fitness if population[0].fitness > 0 else 1.0
                num_clones = max(1, int(self.clone_factor * fit_ratio))
                
                # Clone and mutate
                for _ in range(num_clones):
                    clone = self.mutate_solution(solution)
                    new_population.append(clone)
            
            # Fill remaining with random solutions (diversity)
            while len(new_population) < self.pop_size:
                new_population.append(self.generate_random_solution())
            
            population = new_population
        
        print("-" * 60)
        print(f"Optimization complete!")
        print(f"Best fitness: {best_fitness:.4f}")
        print(f"Best average delay: {best_solution.avg_delay:.2f}ms")
        print(f"Best load imbalance: {best_solution.load_imbalance:.2f}")
        print(f"Edge A load: {best_solution.edge_a_load:.2f} kbps")
        print(f"Edge B load: {best_solution.edge_b_load:.2f} kbps")
        
        return best_solution, self.generation_history


def generate_synthetic_network(num_peers: int = 160, seed: int = 42) -> Tuple[List[Peer], RTTMatrix]:
    """
    Generate a synthetic network topology for simulation
    
    Args:
        num_peers: Number of peer nodes
        seed: Random seed for reproducibility
    
    Returns:
        (peers, rtt_matrix)
    """
    random.seed(seed)
    np.random.seed(seed)
    
    regions = ['us-east', 'us-west', 'eu-west', 'eu-central', 'asia-pacific', 
               'asia-southeast', 'india', 'australia', 'brazil', 'canada']
    
    peers = []
    rtt_matrix = RTTMatrix(peer_to_peer={}, peer_to_edge={})
    
    # Generate peers
    for i in range(1, num_peers + 1):
        peer_id = f"peer-{i}"
        region = random.choice(regions)
        
        # Generate RTT to edges (biased by region)
        # US regions closer to Edge A, EU/Asia closer to Edge B
        if region in ['us-east', 'us-west', 'canada', 'brazil']:
            rtt_a = random.uniform(15, 50)
            rtt_b = random.uniform(80, 150)
        elif region in ['eu-west', 'eu-central']:
            rtt_a = random.uniform(70, 120)
            rtt_b = random.uniform(20, 45)
        else:  # Asia, India, Australia
            rtt_a = random.uniform(100, 200)
            rtt_b = random.uniform(30, 80)
        
        # Add some noise
        rtt_a += random.uniform(-5, 5)
        rtt_b += random.uniform(-5, 5)
        rtt_a = max(10, rtt_a)
        rtt_b = max(10, rtt_b)
        
        # Demand (traffic rate) - some peers are more active
        demand = random.uniform(50, 500)  # kbps
        
        # Generate neighbors (30% connection probability)
        neighbors = []
        for j in range(1, num_peers + 1):
            if i != j and random.random() < 0.3:
                neighbors.append(f"peer-{j}")
        
        peer = Peer(
            id=peer_id,
            region=region,
            demand=demand,
            rtt_to_edge_a=rtt_a,
            rtt_to_edge_b=rtt_b,
            neighbors=neighbors
        )
        peers.append(peer)
        
        # Store edge RTTs
        rtt_matrix.peer_to_edge[(peer_id, 'A')] = rtt_a
        rtt_matrix.peer_to_edge[(peer_id, 'B')] = rtt_b
    
    # Generate peer-to-peer RTTs (based on region proximity)
    for i, peer_i in enumerate(peers):
        for j, peer_j in enumerate(peers):
            if i != j:
                # Base RTT on region
                if peer_i.region == peer_j.region:
                    base_rtt = random.uniform(10, 30)
                elif peer_i.region.split('-')[0] == peer_j.region.split('-')[0]:  # Same continent
                    base_rtt = random.uniform(30, 80)
                else:
                    base_rtt = random.uniform(80, 200)
                
                rtt_matrix.peer_to_peer[(peer_i.id, peer_j.id)] = base_rtt
    
    return peers, rtt_matrix


if __name__ == "__main__":
    # Generate synthetic network
    print("Generating synthetic network...")
    peers, rtt_matrix = generate_synthetic_network(num_peers=160, seed=42)
    print(f"Generated {len(peers)} peers")
    print(f"Generated {len(rtt_matrix.peer_to_peer)} peer-to-peer RTT measurements")
    
    # Create algorithm instance
    algorithm = ImmunePlacementAlgorithm(
        peers=peers,
        rtt_matrix=rtt_matrix,
        num_super_peers=15,
        alpha=0.7,  # Prioritize delay
        beta=0.3,   # Also consider load balance
        pop_size=30,
        clone_factor=3,
        mutation_rate=0.15,
        max_generations=100
    )
    
    # Run optimization
    start_time = time.time()
    best_solution, history = algorithm.optimize()
    elapsed_time = time.time() - start_time
    
    print(f"\nOptimization completed in {elapsed_time:.2f} seconds")
    print(f"\nBest Solution:")
    print(f"  Super Peers: {best_solution.super_peers[:10]}...")  # Show first 10
    print(f"  Edge A assignments: {sum(1 for e in best_solution.edge_assignments.values() if e == 'A')}")
    print(f"  Edge B assignments: {sum(1 for e in best_solution.edge_assignments.values() if e == 'B')}")
    
    # Save results
    results = {
        'best_solution': asdict(best_solution),
        'history': history,
        'parameters': {
            'num_peers': len(peers),
            'num_super_peers': algorithm.num_super_peers,
            'alpha': algorithm.alpha,
            'beta': algorithm.beta,
            'pop_size': algorithm.pop_size,
            'max_generations': algorithm.max_generations,
            'elapsed_time': elapsed_time
        }
    }
    
    with open('results.json', 'w') as f:
        json.dump(results, f, indent=2)
    
    print("\nResults saved to results.json")

