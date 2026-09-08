package seed

import (
	"fmt"
	"slices"

	"github.com/meracare/api/internal/care"
)

/*
Who exists, and how they use the app.

Four shapes of use, because four screens' worth of decisions depend on them:
a senior managing their own care sees no filter strip at all; a daughter with
one parent sees no strip either, which is the common case and the one most
easily broken by designing for the others; a son with two parents in two
countries is what the time gutter exists for; and a professional on a round is
the only person for whom worst-first ordering and a restricted permission set
are visible at all.

Everything here is fiction, and deliberately reads as fiction: no address is
real and no medication list is anybody's.
*/

// Persona is somebody who uses the app.
type Persona struct {
	Key   string
	Name  string
	Email string
	// SignsIn is false for a member who is part of the record but has no
	// account to log into — a former helper whose revoked membership still has
	// to have an author.
	SignsIn bool
}

// Member is one person's place in one circle.
type Member struct {
	PersonaKey string
	Role       care.Role
	// active or revoked. A pending member is an invitation, not a membership.
	Status string
}

// Invitation is a seat in a circle that nobody has taken yet.
type Invitation struct {
	Email string
	Role  care.Role
}

// Circle is one senior and everybody around them.
type Circle struct {
	Key         string
	Name        string
	Timezone    string
	DateOfBirth string
	Phone       string
	Address     string
	Emergency   string

	// SelfKey names the persona who *is* this senior, when they hold their own
	// account. Empty for a senior who never signs in, which is most of them.
	SelfKey string
	// OwnerKey is who created the profile.
	OwnerKey string

	Archived    bool
	Members     []Member
	Invitations []Invitation
}

// Plan is the whole cast.
type Plan struct {
	Personas []Persona
	Circles  []Circle
}

// NewPlan builds the standard cast, addressed at one email domain.
func NewPlan(domain string) Plan {
	at := func(local string) string { return local + "@" + domain }

	personas := []Persona{
		{Key: "solo", Name: "Zahra Iqbal", Email: at("zahra"), SignsIn: true},
		{Key: "daughter", Name: "Sana Ahmed", Email: at("sana"), SignsIn: true},
		{Key: "son", Name: "Bilal Ahmed", Email: at("bilal"), SignsIn: true},
		{Key: "nurse", Name: "Fatima Noor", Email: at("fatima"), SignsIn: true},
		// A former member. He has no account because he no longer signs in, but
		// his row has to exist: care history keeps its author, and a revoked
		// membership is updated rather than deleted.
		{Key: "former", Name: "Naveed Anwar", Email: at("naveed"), SignsIn: false},
	}

	family := care.RoleFamilyMember
	professional := care.RoleProfessionalCaregiver

	circles := []Circle{
		{
			Key: "zahra", Name: "Zahra Iqbal", Timezone: "Asia/Karachi",
			DateOfBirth: "1954-06-19", Phone: "+92 300 1112233",
			Address: "12-B Gulberg III, Lahore", Emergency: "Imran Iqbal · +92 321 4455667",
			SelfKey: "solo", OwnerKey: "solo",
			Members: []Member{{PersonaKey: "solo", Role: care.RoleSenior, Status: "active"}},
		},
		{
			// The busy circle: two relatives, a paid caregiver, somebody who
			// left, and a seat still open. Every state the members screen can
			// draw is in this one place.
			Key: "amina", Name: "Amina Bibi", Timezone: "Asia/Karachi",
			DateOfBirth: "1949-03-12", Phone: "+92 301 9988776",
			Address: "House 4, Street 11, F-8/3, Islamabad", Emergency: "Sana Ahmed · +92 333 1234567",
			OwnerKey: "daughter",
			Members: []Member{
				{PersonaKey: "daughter", Role: family, Status: "active"},
				{PersonaKey: "son", Role: family, Status: "active"},
				{PersonaKey: "nurse", Role: professional, Status: "active"},
				{PersonaKey: "former", Role: family, Status: "revoked"},
			},
			Invitations: []Invitation{{Email: at("rabia"), Role: family}},
		},
		{
			// London, so the son's day holds two clocks at once.
			Key: "yusuf", Name: "Yusuf Khan", Timezone: "Europe/London",
			DateOfBirth: "1946-11-02", Phone: "+44 7700 900123",
			Address: "41 Beckton Road, London E16", Emergency: "Bilal Ahmed · +92 345 7654321",
			OwnerKey: "son",
			Members:  []Member{{PersonaKey: "son", Role: family, Status: "active"}},
		},
		{
			Key: "bilal", Name: "Bilal Ahmed", Timezone: "Asia/Karachi",
			DateOfBirth: "1981-01-25", Phone: "+92 345 7654321",
			SelfKey: "son", OwnerKey: "son",
			Members: []Member{{PersonaKey: "son", Role: care.RoleSenior, Status: "active"}},
		},
		{
			Key: "iqbal", Name: "Iqbal Hussain", Timezone: "Asia/Karachi",
			DateOfBirth: "1943-08-30", Phone: "+92 302 5566778",
			Address: "Flat 7, Askari IV, Karachi", Emergency: "Nadia Hussain · +92 300 8877665",
			OwnerKey: "nurse",
			Members:  []Member{{PersonaKey: "nurse", Role: professional, Status: "active"}},
		},
		{
			Key: "rukhsana", Name: "Rukhsana Bibi", Timezone: "Asia/Karachi",
			DateOfBirth: "1951-12-04", Phone: "+92 306 2233445",
			Address: "88 Model Town, Lahore", Emergency: "Asif Raza · +92 322 9988771",
			OwnerKey: "nurse",
			Members:  []Member{{PersonaKey: "nurse", Role: professional, Status: "active"}},
		},
		{
			Key: "ghulam", Name: "Ghulam Rasool", Timezone: "Asia/Karachi",
			DateOfBirth: "1940-05-17", Phone: "+92 308 1122334",
			Address: "House 219, Satellite Town, Rawalpindi", Emergency: "Shahid Rasool · +92 331 4433221",
			OwnerKey: "nurse",
			Members:  []Member{{PersonaKey: "nurse", Role: professional, Status: "active"}},
		},
		{
			// Archived: care that has ended. It must not appear beside the
			// living circles, and the only way to know it does not is to have
			// one.
			Key: "naseem", Name: "Naseem Akhtar", Timezone: "Asia/Karachi",
			DateOfBirth: "1938-02-08", OwnerKey: "nurse", Archived: true,
			Members: []Member{{PersonaKey: "nurse", Role: professional, Status: "active"}},
		},
	}

	return Plan{Personas: personas, Circles: circles}
}

// Persona finds one by key.
func (p Plan) Persona(key string) (Persona, bool) {
	i := slices.IndexFunc(p.Personas, func(persona Persona) bool { return persona.Key == key })
	if i < 0 {
		return Persona{}, false
	}
	return p.Personas[i], true
}

// Circle finds one by key.
func (p Plan) Circle(key string) (Circle, bool) {
	i := slices.IndexFunc(p.Circles, func(circle Circle) bool { return circle.Key == key })
	if i < 0 {
		return Circle{}, false
	}
	return p.Circles[i], true
}

// CirclesFor lists the circles a persona actually sees in the app: the ones
// they still belong to, whose senior has not been archived.
func (p Plan) CirclesFor(personaKey string) []Circle {
	var found []Circle

	for _, circle := range p.Circles {
		if circle.Archived {
			continue
		}
		for _, member := range circle.Members {
			if member.PersonaKey == personaKey && member.Status == "active" {
				found = append(found, circle)
				break
			}
		}
	}

	return found
}

// Membership finds one person's place in one circle.
func (p Plan) Membership(personaKey, circleKey string) (Member, bool) {
	circle, ok := p.Circle(circleKey)
	if !ok {
		return Member{}, false
	}

	for _, member := range circle.Members {
		if member.PersonaKey == personaKey {
			return member, true
		}
	}

	return Member{}, false
}

// Validate checks the plan against the rules the database also enforces, so a
// mistake in the cast is a readable message rather than a constraint violation
// halfway through a write.
func (p Plan) Validate() error {
	keys := map[string]bool{}
	for _, persona := range p.Personas {
		if keys[persona.Key] {
			return fmt.Errorf("seed: two personas share the key %q", persona.Key)
		}
		keys[persona.Key] = true
	}

	seen := map[string]bool{}
	for _, circle := range p.Circles {
		if seen[circle.Key] {
			return fmt.Errorf("seed: two circles share the key %q", circle.Key)
		}
		seen[circle.Key] = true

		if _, ok := p.Persona(circle.OwnerKey); !ok {
			return fmt.Errorf("seed: circle %q is owned by unknown persona %q", circle.Key, circle.OwnerKey)
		}
		if circle.SelfKey != "" {
			if _, ok := p.Persona(circle.SelfKey); !ok {
				return fmt.Errorf("seed: circle %q is unknown persona %q", circle.Key, circle.SelfKey)
			}
		}

		var seniors int
		members := map[string]bool{}

		for _, member := range circle.Members {
			if _, ok := p.Persona(member.PersonaKey); !ok {
				return fmt.Errorf("seed: circle %q has unknown member %q", circle.Key, member.PersonaKey)
			}
			if members[member.PersonaKey] {
				return fmt.Errorf("seed: %q is in circle %q twice", member.PersonaKey, circle.Key)
			}
			members[member.PersonaKey] = true

			if !member.Role.Valid() {
				return fmt.Errorf("seed: circle %q gives %q the unknown role %q", circle.Key, member.PersonaKey, member.Role)
			}
			if member.Status != "active" && member.Status != "revoked" {
				return fmt.Errorf("seed: circle %q gives %q the unknown status %q", circle.Key, member.PersonaKey, member.Status)
			}
			// The database holds a unique index on one senior per circle.
			if member.Role == care.RoleSenior && member.Status != "revoked" {
				seniors++
			}
		}

		if seniors > 1 {
			return fmt.Errorf("seed: circle %q has %d seniors", circle.Key, seniors)
		}
		if circle.SelfKey != "" && !members[circle.SelfKey] {
			return fmt.Errorf("seed: circle %q is %q but they are not in it", circle.Key, circle.SelfKey)
		}

		for _, invitation := range circle.Invitations {
			if !care.CanInviteRole(invitation.Role) {
				return fmt.Errorf("seed: circle %q invites into the uninvitable role %q", circle.Key, invitation.Role)
			}
		}
	}

	return nil
}
