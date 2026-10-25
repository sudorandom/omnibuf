@0xd94a71916f22a8c3;

struct Location {
  region     @0 :Text;
  datacenter @1 :Text;
  city       @2 :Text;
  rack       @3 :UInt32;
  latitude   @4 :Float32;
  longitude  @5 :Float32;
}

struct Neighbor {
  peerAsn     @0 :UInt32;
  router      @1 :Text;
  latencyUs   @2 :UInt32;
  established @3 :Bool;
}

struct PeerUpdate {
  asn       @0  :UInt32;
  routerId  @1  :Text;
  active    @2  :Bool;
  uptimeSec @3  :UInt64;
  rssiDbm   @4  :Int32;
  cpuLoad   @5  :Float64;
  tags      @6  :List(Text);
  prefixes  @7  :List(UInt32);
  location  @8  :Location;
  neighbors @9  :List(Neighbor);
  sequence  @10 :UInt64;
}
